package realtime

import (
	"context"
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

	turnLat        *TurnLatency
	turnAudioBytes int
	preferMP3      bool
	echoGuardMS    int // 0 = 使用服务端默认

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

// SetTurnPCM 保存本 turn 的用户 PCM（供声学情绪旁路）。
func (s *Session) SetTurnPCM(pcm []byte) {
	s.mu.Lock()
	s.turnPCM = append([]byte(nil), pcm...)
	s.acousticReady = false
	s.acousticHint = emotion.EmptyAcousticHint()
	s.visualReady = false
	s.visualHint = vision.EmptyHint()
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
