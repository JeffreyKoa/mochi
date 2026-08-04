package realtime

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/vision"
)

type SessionState string

const (
	StateIdle      SessionState = "idle"
	StateListening SessionState = "listening"
	StateThinking  SessionState = "thinking"
	StateSpeaking  SessionState = "speaking"
)

type Session struct {
	ID     string
	UserID uint64

	mu    sync.Mutex
	state SessionState

	audioSeq int64
	ttsSeq   int64

	pipelineMu     sync.Mutex
	pipelineCancel context.CancelFunc

	visionWorkMu  sync.Mutex
	visionCancel  context.CancelFunc
	visionWorkCtx context.Context

	turnLat        *TurnLatency
	turnAudioBytes int
	preferMP3      bool
	localTTS       bool // client_caps：客户端本地 TTS，服务端跳过 DashScope TTS
	echoGuardMS    int // 0 = 使用服务端默认

	topicAnchor TopicAnchor

	turnPCM        []byte
	acousticHint   emotion.AcousticHint
	acousticReady  bool

	turnVisionJPEG []byte
	visualHint     vision.Hint
	visualReady    bool
	visionPrefetching bool
	lastVisionFrameAt time.Time
	lastVisionSeq     int64
	lastVisionReason  string

	// P2：客户端 face_probe 推断的视觉说话人 owner|unknown
	visualSpeaker   string
	visualSpeakerAt time.Time

	// Phase C：THINK 阶段 Tier-0 GLANCE（不打断 LLM）
	glanceMu         sync.Mutex
	pendingGlance    vision.Hint
	glanceNextTurn   vision.Hint
	llmStreaming     bool

	onStateChange func(SessionState)
}

func NewSession(id string, userID uint64, onStateChange func(SessionState)) *Session {
	return &Session{
		ID:            id,
		UserID:        userID,
		state:         StateIdle,
		onStateChange: onStateChange,
	}
}

func (s *Session) State() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) SetState(next SessionState) {
	s.mu.Lock()
	if s.state == next {
		s.mu.Unlock()
		return
	}
	s.state = next
	cb := s.onStateChange
	s.mu.Unlock()
	if cb != nil {
		cb(next)
	}
}

func (s *Session) NextAudioSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audioSeq++
	return s.audioSeq
}

func (s *Session) NextTTSSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttsSeq++
	return s.ttsSeq
}

// BeginPipeline returns a cancellable context for an active reply pipeline.
func (s *Session) BeginPipeline(parent context.Context) context.Context {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	if s.pipelineCancel != nil {
		s.pipelineCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.pipelineCancel = cancel
	return ctx
}

// CancelPipeline stops the active reply pipeline (barge-in).
func (s *Session) CancelPipeline() {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	if s.pipelineCancel != nil {
		s.pipelineCancel()
		s.pipelineCancel = nil
	}
	s.CancelVisionWork()
}

// EndPipeline clears the pipeline cancel handle after normal completion.
func (s *Session) EndPipeline() {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	s.pipelineCancel = nil
}

func (s *Session) BeginTurn(origin time.Time) *TurnLatency {
	lat := NewTurnLatency(origin)
	s.mu.Lock()
	s.turnLat = lat
	s.mu.Unlock()
	return lat
}

func (s *Session) TurnLatency() *TurnLatency {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnLat
}

func (s *Session) ClearTurnLatency() {
	s.mu.Lock()
	s.turnLat = nil
	s.mu.Unlock()
}

func (s *Session) SetTurnAudioBytes(n int) {
	s.mu.Lock()
	s.turnAudioBytes = n
	s.mu.Unlock()
}

func (s *Session) TurnAudioBytes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnAudioBytes
}

func (s *Session) SetPreferMP3(v bool) {
	s.mu.Lock()
	s.preferMP3 = v
	s.mu.Unlock()
}

func (s *Session) PreferMP3() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preferMP3
}

func (s *Session) SetLocalTTS(v bool) {
	s.mu.Lock()
	s.localTTS = v
	s.mu.Unlock()
}

func (s *Session) LocalTTS() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localTTS
}

// SetTurnPCM 保存本 turn 的用户 PCM（供声学情绪旁路）。
// 不清除 visualReady/visualHint：speech_start prefetch 的结果应保留到 audio_end 复用。
func (s *Session) SetTurnPCM(pcm []byte) {
	s.mu.Lock()
	s.turnPCM = append([]byte(nil), pcm...)
	s.acousticReady = false
	s.acousticHint = emotion.EmptyAcousticHint()
	s.mu.Unlock()
}

// TurnPCM 返回本 turn 用户 PCM 副本。
func (s *Session) TurnPCM() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.turnPCM...)
}

// SetAcousticHint 缓存声学识别结果。
func (s *Session) SetAcousticHint(h emotion.AcousticHint) {
	s.mu.Lock()
	s.acousticHint = h
	s.acousticReady = true
	s.mu.Unlock()
}

// AcousticHint 返回本 turn 声学情绪（未识别则 empty）。
func (s *Session) AcousticHint() emotion.AcousticHint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acousticHint
}

func (s *Session) acousticDone() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acousticReady
}

// SetVisionFrame 缓存本 turn 客户端上传的 JPEG（后帧覆盖前帧）。
// speech_start 才重置 prefetch；object_refresh/audio_end 只更新 JPEG，保留最新画面给 contextual VL。
func (s *Session) SetVisionFrame(jpeg []byte, seq int64, reason string) {
	s.mu.Lock()
	s.turnVisionJPEG = append([]byte(nil), jpeg...)
	s.lastVisionFrameAt = time.Now()
	s.lastVisionSeq = seq
	s.lastVisionReason = reason
	// 举物补帧 / 提交帧不应打断 owner_face prefetch，也不应重复触发 prefetch
	if reason == "" || reason == "speech_start" {
		s.visualReady = false
		s.visualHint = vision.EmptyHint()
		s.visionPrefetching = false
	}
	s.mu.Unlock()
}

// HasVisionFrame 本 turn 是否已有视觉帧。
func (s *Session) HasVisionFrame() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.turnVisionJPEG) > 0
}

// TurnVisionJPEG 返回本 turn JPEG 副本。
func (s *Session) TurnVisionJPEG() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.turnVisionJPEG...)
}

// ClearVisionFrame 消费后清除帧缓存。
func (s *Session) ClearVisionFrame() {
	s.mu.Lock()
	s.turnVisionJPEG = nil
	s.mu.Unlock()
}

// SetVisualHint 缓存视觉识别结果。
func (s *Session) SetVisualHint(h vision.Hint) {
	s.mu.Lock()
	s.visualHint = h
	s.visualReady = true
	s.mu.Unlock()
}

// VisualHint 返回本 turn 视觉结果。
func (s *Session) VisualHint() vision.Hint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visualHint
}

func (s *Session) visualDone() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visualReady
}

// TryBeginVisionPrefetch 尝试启动 vision_frame 预分析（同一 turn 仅一次）。
func (s *Session) TryBeginVisionPrefetch() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visualReady || s.visionPrefetching || len(s.turnVisionJPEG) == 0 {
		return false
	}
	s.visionPrefetching = true
	return true
}

// EndVisionPrefetch 预分析结束（成功或失败均需调用）。
func (s *Session) EndVisionPrefetch() {
	s.mu.Lock()
	s.visionPrefetching = false
	s.mu.Unlock()
}

// IsVisionPrefetching 是否正在预分析。
func (s *Session) IsVisionPrefetching() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visionPrefetching
}

// WaitForVisualHint 等待 prefetch 完成（V3a 流式路径避免重复 VL）。
func (s *Session) WaitForVisualHint(ctx context.Context, timeout time.Duration) vision.Hint {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.visualDone() {
			return s.VisualHint()
		}
		if !s.IsVisionPrefetching() {
			break
		}
		select {
		case <-ctx.Done():
			return vision.EmptyHint()
		case <-time.After(10 * time.Millisecond):
		}
	}
	if s.visualDone() {
		return s.VisualHint()
	}
	return vision.EmptyHint()
}

// ClearTurnMedia 新一轮 utterance 开始时清除音视频缓存。
func (s *Session) ClearTurnMedia() {
	s.CancelVisionWork()
	s.ResetGlanceTurn()
	s.mu.Lock()
	s.turnPCM = nil
	s.turnVisionJPEG = nil
	s.acousticReady = false
	s.acousticHint = emotion.EmptyAcousticHint()
	s.visualReady = false
	s.visualHint = vision.EmptyHint()
	s.visionPrefetching = false
	s.mu.Unlock()
}

// ResetVisionWork 新一轮 LISTEN（speech_start）时绑定可取消的视觉 goroutine 上下文。
func (s *Session) ResetVisionWork(parent context.Context) context.Context {
	s.visionWorkMu.Lock()
	defer s.visionWorkMu.Unlock()
	if s.visionCancel != nil {
		s.visionCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.visionCancel = cancel
	s.visionWorkCtx = ctx
	return ctx
}

// CancelVisionWork 打断/Skip/barge-in 时取消在途 Tier-1 VL。
func (s *Session) CancelVisionWork() {
	s.visionWorkMu.Lock()
	defer s.visionWorkMu.Unlock()
	if s.visionCancel != nil {
		s.visionCancel()
		s.visionCancel = nil
	}
	s.visionWorkCtx = nil
}

// VisionWorkCtx 返回当前 turn 视觉 ctx；未初始化时用 fallback。
func (s *Session) VisionWorkCtx(fallback context.Context) context.Context {
	s.visionWorkMu.Lock()
	defer s.visionWorkMu.Unlock()
	if s.visionWorkCtx != nil {
		return s.visionWorkCtx
	}
	return fallback
}

// LastVisionReason 返回最近一帧 reason。
func (s *Session) LastVisionReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastVisionReason
}

// WaitForVisionFrameReason 硬焦点 Barrier：等待 audio_end / object_refresh 帧。
func (s *Session) WaitForVisionFrameReason(ctx context.Context, reasons []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	want := make(map[string]bool, len(reasons))
	for _, r := range reasons {
		want[r] = true
	}
	for time.Now().Before(deadline) {
		s.mu.Lock()
		reason := s.lastVisionReason
		hasFrame := len(s.turnVisionJPEG) > 0
		s.mu.Unlock()
		if hasFrame && (len(want) == 0 || want[reason]) {
			return true
		}
		select {
		case <-ctx.Done():
			return s.HasVisionFrame()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return s.HasVisionFrame()
}

// SetEchoGuardMS 设置本会话 effective echo guard（client_caps AEC 握手）。
func (s *Session) SetEchoGuardMS(ms int) {
	s.mu.Lock()
	s.echoGuardMS = ms
	s.mu.Unlock()
}

// EffectiveEchoGuardMS 返回会话级 echo guard，0 表示使用 caller 传入的默认值。
func (s *Session) EffectiveEchoGuardMS(defaultMS int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.echoGuardMS > 0 {
		return s.echoGuardMS
	}
	return defaultMS
}

// TopicAnchor 返回当前会话话题锚点副本。
func (s *Session) TopicAnchor() TopicAnchor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.topicAnchor
}

// ApplyTopicAnchor 根据本 turn classify 结果更新锚点并返回快照。
func (s *Session) ApplyTopicAnchor(userText string, insight emotion.UtteranceInsight, cfg TopicAnchorConfig) TopicAnchor {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topicAnchor = UpdateTopicAnchor(s.topicAnchor, userText, insight, cfg)
	return s.topicAnchor
}

// ApplyFaceProbe 更新会话级 visual_speaker（P2，TTL 由 caller 在读取时校验）。
// 低分=看不清脸不更新；仅高置信 non-match 才标 unknown，避免单帧 crop 误伤。
func (s *Session) ApplyFaceProbe(match bool, score float64, detected bool) {
	if !detected {
		return
	}
	const minUnknown = 0.32
	s.mu.Lock()
	defer s.mu.Unlock()
	if match {
		s.visualSpeaker = "owner"
		s.visualSpeakerAt = time.Now()
		return
	}
	if score >= minUnknown {
		s.visualSpeaker = "unknown"
		s.visualSpeakerAt = time.Now()
	}
}

// VisualSpeakerForPrompt 返回仍在 TTL 内的 visual_speaker，否则空串。
func (s *Session) VisualSpeakerForPrompt(ownerRecentMS int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visualSpeaker == "" {
		return ""
	}
	if ownerRecentMS <= 0 {
		ownerRecentMS = 8000
	}
	if time.Since(s.visualSpeakerAt) > time.Duration(ownerRecentMS)*time.Millisecond {
		return ""
	}
	return s.visualSpeaker
}

// SetLLMStreaming 标记当前 turn LLM 是否已开始出 token（GLANCE 消费规则 §2.5）。
func (s *Session) SetLLMStreaming(v bool) {
	s.glanceMu.Lock()
	s.llmStreaming = v
	s.glanceMu.Unlock()
}

// LLMStreaming 是否已 streaming。
func (s *Session) LLMStreaming() bool {
	s.glanceMu.Lock()
	defer s.glanceMu.Unlock()
	return s.llmStreaming
}

// ApplyGlanceHint Tier-0 GLANCE：LLM 已 streaming 则仅写入下轮，不重生成。
func (s *Session) ApplyGlanceHint(h vision.Hint) {
	s.glanceMu.Lock()
	defer s.glanceMu.Unlock()
	if s.llmStreaming {
		s.glanceNextTurn = h
		log.Printf("[vision][glance] store=next_turn focus=%s expr=%s", h.Focus, h.UserExpression)
		return
	}
	s.pendingGlance = h
	log.Printf("[vision][glance] store=pending focus=%s expr=%s", h.Focus, h.UserExpression)
}

// TakePendingGlance 取出尚未消费的本 turn GLANCE（TTS 首句前可影响 prosody）。
func (s *Session) TakePendingGlance() vision.Hint {
	s.glanceMu.Lock()
	defer s.glanceMu.Unlock()
	h := s.pendingGlance
	s.pendingGlance = vision.EmptyHint()
	return h
}

// TakeGlanceForNextTurn 取出写入下轮记忆的 GLANCE。
func (s *Session) TakeGlanceForNextTurn() vision.Hint {
	s.glanceMu.Lock()
	defer s.glanceMu.Unlock()
	h := s.glanceNextTurn
	s.glanceNextTurn = vision.EmptyHint()
	return h
}

// ResetGlanceTurn 新 turn 开始时清理 GLANCE 状态。
func (s *Session) ResetGlanceTurn() {
	s.glanceMu.Lock()
	s.pendingGlance = vision.EmptyHint()
	s.llmStreaming = false
	s.glanceMu.Unlock()
}
