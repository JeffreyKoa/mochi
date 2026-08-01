package realtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/agent"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/text"
	"github.com/mochi-ai/server/internal/vision"
	"github.com/mochi-ai/server/pkg/dashscope"
	"github.com/mochi-ai/server/pkg/opus"
)

// Pipeline orchestrates ASR → LLM → TTS (turn-based, half-duplex).
type Pipeline struct {
	chat      *chat.Service
	cfg       config.RealtimeConfig
	asr       ASRRecognizer
	tts       TTSSynthesizer
	ttsFormat string
	apiKey    string
	ep        dashscope.EndpointConfig
	gate      *ResponseGate
	noiseFillers map[rune]bool
	acoustic     emotion.AcousticClient
	vision       *vision.Service
	asrSampleRate int
}

func NewPipeline(chatSvc *chat.Service, cfg config.RealtimeConfig, appCfg *config.Config) *Pipeline {
	apiKey := appCfg.AI.APIKey
	ep := dashscope.EndpointConfig{
		WSURL:       cfg.Dashscope.WSURL,
		WorkspaceID: cfg.Dashscope.WorkspaceID,
		Region:      cfg.Dashscope.Region,
	}
	p := &Pipeline{
		chat:          chatSvc,
		cfg:           cfg,
		ttsFormat:     "mp3",
		apiKey:        apiKey,
		ep:            ep,
		asrSampleRate: cfg.ASR.SampleRate,
		acoustic:      emotion.NoopAcousticClient{},
	}
	if appCfg.Emotion.Acoustic.Enabled && appCfg.Emotion.Acoustic.URL != "" {
		p.acoustic = emotion.NewHTTPAcousticClient(
			appCfg.Emotion.Acoustic.URL,
			time.Duration(appCfg.Emotion.Acoustic.TimeoutMS)*time.Millisecond,
			cfg.ASR.SampleRate,
		)
	}
	p.vision = vision.NewService(appCfg.AI, appCfg.Vision)
	if p.vision != nil && p.vision.Enabled() {
		log.Printf("[vision] pipeline enabled model=%s timeout_ms=%d min_conf=%.2f prefetch=%v parallel=%v early_anim=%v",
			appCfg.Vision.Model, appCfg.Vision.TimeoutMS, appCfg.Vision.MinExpressionConfidence,
			appCfg.Vision.PrefetchOnFrame, !appCfg.Vision.SequentialOwnerFace, appCfg.Vision.EarlyAnimation)
	} else {
		log.Printf("[vision] pipeline disabled (config vision.enabled=false)")
	}
	asrEp := dashscope.EndpointConfig{WSURL: cfg.Dashscope.ASRWSURL}
	if cfg.ASR.Provider == "dashscope" && apiKey != "" {
		p.asr = newDashscopeASR(dashscope.NewASRClient(apiKey, cfg.ASR.Model, cfg.ASR.SampleRate, asrEp))
	}

	p.tts, p.ttsFormat = buildTTSSynth(cfg, apiKey, ep, ttsPreferMP3(cfg, false))
	p.gate = NewResponseGate(cfg.Gate, appCfg.GateFastpath, appCfg.GateSystemPrompt, apiKey, appCfg.AI.APIBase)
	p.noiseFillers = appCfg.NoiseFillers
	return p
}

func ttsPreferMP3(cfg config.RealtimeConfig, clientPreferMP3 bool) bool {
	if clientPreferMP3 {
		return true
	}
	if strings.ToLower(cfg.TTS.Transport) != "opus" {
		return true
	}
	return !opus.Available()
}

// sessionTTSBundle 含会话级 TTS 合成器、格式与音色基线。
type sessionTTSBundle struct {
	synth    TTSSynthesizer
	format   string
	baseline VoiceProfile
}

// ttsSegment 为带 prosody 的分句 TTS 任务。
type ttsSegment struct {
	text string
	opts dashscope.SynthOptions
}

func (p *Pipeline) getTTSForSession(ctx context.Context, sess *Session) sessionTTSBundle {
	preferMP3 := ttsPreferMP3(p.cfg, sess.PreferMP3())
	empty := sessionTTSBundle{format: "mp3", baseline: VoiceProfile{Rate: "+0%", Pitch: "+0Hz"}}
	if p.tts == nil {
		return empty
	}
	if p.chat == nil {
		return sessionTTSBundle{synth: p.tts, format: p.ttsFormat, baseline: empty.baseline}
	}
	pet, err := p.chat.GetPetByUser(ctx, sess.UserID)
	if err != nil || pet == nil {
		return sessionTTSBundle{synth: p.tts, format: p.ttsFormat, baseline: empty.baseline}
	}

	profile := ResolveVoice(pet.Gender, pet.LifeStage, string(pet.PersonalityJSON))
	cfg := p.cfg
	if profile.DashscopeVoice != "" {
		cfg.TTS.Voice = profile.DashscopeVoice
	}

	synth, fmtStr := buildTTSSynth(cfg, p.apiKey, p.ep, preferMP3)
	if synth != nil {
		return sessionTTSBundle{synth: synth, format: fmtStr, baseline: profile}
	}
	return sessionTTSBundle{synth: p.tts, format: p.ttsFormat, baseline: profile}
}

func buildTTSSynth(cfg config.RealtimeConfig, apiKey string, ep dashscope.EndpointConfig, preferMP3 bool) (TTSSynthesizer, string) {
	format := "mp3"
	if apiKey == "" {
		return nil, format
	}

	client := dashscope.NewTTSClient(apiKey, cfg.TTS.Model, cfg.TTS.Voice, cfg.TTS.SampleRate, ep)
	useOpusPath := strings.ToLower(cfg.TTS.Transport) == "opus" && !preferMP3 && opus.Available()
	if useOpusPath {
		client.SetAudioFormat("pcm")
		format = "pcm"
	} else {
		if strings.ToLower(cfg.TTS.Transport) == "opus" && !opus.Available() {
			log.Printf("[realtime] opus encoder unavailable, tts will use mp3")
		}
		format = client.AudioFormat()
	}
	return newDashscopeTTSSynth(client), format
}

func (p *Pipeline) StartASRSession(ctx context.Context, onPartial ASRPartialHandler) (ASRSession, error) {
	if p.asr == nil {
		return nil, fmt.Errorf("asr not configured")
	}
	return p.asr.StartSession(ctx, onPartial)
}

// PrewarmTTS primes the TTS provider with a minimal synthesis (best-effort).
func (p *Pipeline) PrewarmTTS(ctx context.Context) {
	if p.tts == nil {
		return
	}
	go func() {
		warmCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := p.tts.Synthesize(warmCtx, "嗯", dashscope.DefaultSynthOptions(), func([]byte) {}); err != nil {
			log.Printf("[realtime] tts prewarm: %v", err)
			return
		}
		log.Printf("[realtime] tts prewarm ok")
	}()
}

func (p *Pipeline) OnSpeechEnd(ctx context.Context, sess *Session, audio []byte, send Sender) {
	sess.SetState(StateThinking)
	send.SendAnimation(StateThinking)

	log.Printf("[realtime] session=%s user=%d audio_bytes=%d", sess.ID, sess.UserID, len(audio))
	sess.SetTurnPCM(audio)
	sess.SetTurnAudioBytes(len(audio))

	if p.asr == nil {
		p.failTurn(ctx, sess, send, "ASR_NOT_CONFIGURED", "ASR 未配置，请在 config.yaml 设置 ai.api_key")
		return
	}

	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()

	var (
		text         string
		asrErr       error
		acousticHint emotion.AcousticHint
		faceHint     vision.Hint
		lastPartial  string
		faceMu       sync.Mutex
	)

	log.Printf("[perception] parallel_start session=%s has_vision=%v", sess.ID, sess.HasVisionFrame())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		text, asrErr = p.asr.Recognize(pipeCtx, audio, func(partial string, _ bool) {
			if partial == lastPartial {
				return
			}
			lastPartial = partial
			_ = send.Send(MsgASRPartial, ASRText{Text: partial})
		})
	}()

	if p.acoustic != nil && p.acoustic.Enabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hint, err := p.acoustic.Recognize(pipeCtx, audio, p.asrSampleRate)
			if err != nil {
				log.Printf("[realtime] acoustic error session=%s: %v", sess.ID, err)
				return
			}
			acousticHint = hint
			log.Printf("[realtime] acoustic session=%s mood=%s conf=%.2f", sess.ID, hint.Mood, hint.Confidence)
		}()
	}

	// V3a：owner_face 与 ASR 并行（prefetch 已完成则复用）
	if p.autoVisionOnVoiceTurn() && sess.HasVisionFrame() {
		if sess.visualDone() {
			faceHint = sess.VisualHint()
			log.Printf("[realtime][vision] prefetch_hit session=%s focus=%s", sess.ID, faceHint.Focus)
		} else if p.vision != nil && p.vision.ParallelOwnerFace() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h := p.awaitOwnerFaceHint(pipeCtx, sess)
				if !sess.visualDone() && h.Focus != vision.FocusSkip {
					sess.SetVisualHint(h)
				}
				faceMu.Lock()
				faceHint = h
				faceMu.Unlock()
			}()
		}
	}

	wg.Wait()

	if asrErr != nil {
		if p.handleCancelled(pipeCtx, sess, send, asrErr) {
			return
		}
		log.Printf("[realtime] asr error session=%s: %v", sess.ID, asrErr)
		p.failTurn(ctx, sess, send, "ASR_FAILED", fmt.Sprintf("语音识别失败: %v", asrErr))
		return
	}

	if text == "" {
		text = lastPartial
	}
	sess.SetAcousticHint(acousticHint)

	faceMu.Lock()
	fh := faceHint
	faceMu.Unlock()
	if !fh.IsUsable() && fh.Focus == "" && sess.visualDone() {
		fh = sess.VisualHint()
	}

	visualHint := p.finalizeParallelPerception(pipeCtx, sess, text, acousticHint, fh)
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkVisionFinal()
		lat.MarkPerceiveParallel()
	}
	log.Printf("[perception] parallel_done session=%s text=%q final_focus=%s mood=%s intent=%s",
		sess.ID, truncateVisionText(text, 40), visualHint.Visual.Focus,
		visualHint.Hint.UserMood, visualHint.Hint.Intent)
	sess.SetVisualHint(visualHint.Visual)
	sess.ClearVisionFrame()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true, visualHint)
}

func (p *Pipeline) OnTranscript(ctx context.Context, sess *Session, text string, send Sender) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true, nil)
}

func (p *Pipeline) OnTextInput(ctx context.Context, sess *Session, text string, send Sender, withVoice bool) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, withVoice, nil)
}

// ensureAcousticHint 对流式 ASR 路径补跑声学识别（OnSpeechEnd 已并行则跳过）。
func (p *Pipeline) ensureAcousticHint(ctx context.Context, sess *Session) emotion.AcousticHint {
	if sess.acousticDone() {
		return sess.AcousticHint()
	}
	pcm := sess.TurnPCM()
	if p.acoustic == nil || !p.acoustic.Enabled() || len(pcm) == 0 {
		return emotion.EmptyAcousticHint()
	}
	hint, err := p.acoustic.Recognize(ctx, pcm, p.asrSampleRate)
	if err != nil {
		log.Printf("[realtime] acoustic fallback error session=%s: %v", sess.ID, err)
		return emotion.EmptyAcousticHint()
	}
	sess.SetAcousticHint(hint)
	log.Printf("[realtime] acoustic(stream) session=%s mood=%s conf=%.2f", sess.ID, hint.Mood, hint.Confidence)
	return hint
}

// PrefetchOwnerFace vision_frame 到达时预跑 owner_face（V3a，与录音并行）。
func (p *Pipeline) PrefetchOwnerFace(ctx context.Context, sess *Session) {
	if p.vision == nil || !p.vision.PrefetchOnFrame() || !sess.TryBeginVisionPrefetch() {
		return
	}
	go func() {
		defer sess.EndVisionPrefetch()
		jpeg := sess.TurnVisionJPEG()
		if len(jpeg) == 0 {
			return
		}
		hint := p.vision.DescribeOwnerFace(ctx, jpeg, sess.ID)
		sess.SetVisualHint(hint)
		log.Printf("[realtime][vision] prefetch_done session=%s focus=%s skipped=%v conf=%.2f",
			sess.ID, hint.Focus, hint.Skipped, hint.ExpressionConfidence)
	}()
}

// finalizeParallelPerception ASR 完成后 classify + contextual refine + 融合（V3c）。
func (p *Pipeline) finalizeParallelPerception(ctx context.Context, sess *Session, userText string, acoustic emotion.AcousticHint, faceHint vision.Hint) *emotion.PerceptionState {
	jpeg := sess.TurnVisionJPEG()
	if p.vision == nil || !p.vision.Enabled() || len(jpeg) == 0 {
		state := emotion.BuildFinalPerception(userText, acoustic, vision.EmptyHint(),
			p.classifyUtterance(ctx, userText, acoustic, faceHint), 0.65, 0.6)
		return &state
	}

	if !p.vision.ParallelOwnerFace() && !faceHint.IsUsable() {
		faceHint = p.vision.DescribeOwnerFace(ctx, jpeg, sess.ID)
	} else if !faceHint.IsUsable() && sess.visualDone() {
		faceHint = sess.VisualHint()
	}

	p.maybeEarlyPerceptionAnim(ctx, sess, acoustic, faceHint)

	if strings.TrimSpace(userText) == "" {
		state := emotion.BuildFinalPerception("", acoustic, faceHint,
			emotion.FallbackInsight(acoustic, faceHint), 0.65, p.vision.MinVisualConf())
		return &state
	}
	return p.buildPerceptionState(ctx, userText, acoustic, faceHint, jpeg, sess.ID)
}

// buildPerceptionState V3c：ClassifyUtterance → RefineContextual → BuildFinalPerception。
func (p *Pipeline) buildPerceptionState(ctx context.Context, userText string, acoustic emotion.AcousticHint, faceHint vision.Hint, jpeg []byte, sessionID string) *emotion.PerceptionState {
	minAcoustic := 0.65
	minVisual := 0.6
	if p.vision != nil {
		minVisual = p.vision.MinVisualConf()
	}

	insight := p.classifyUtterance(ctx, userText, acoustic, faceHint)
	visualHint := faceHint
	if p.vision != nil && len(jpeg) > 0 {
		if p.vision.ContextualPlannerEnabled() {
			visualHint = p.vision.RefineContextual(ctx, jpeg, userText, faceHint, insight.VisualTask, insight.FaceTextClash, sessionID)
		} else {
			visualHint = p.vision.RefineAfterASR(ctx, jpeg, userText, faceHint, sessionID)
		}
	}

	state := emotion.BuildFinalPerception(userText, acoustic, visualHint, insight, minAcoustic, minVisual)
	log.Printf("[perception] final mood=%s intent=%s empathy=%v source=%s visual_task=%s",
		state.Hint.UserMood, state.Hint.Intent, state.Hint.NeedsEmpathy, state.Source, insight.VisualTask)
	return &state
}

func (p *Pipeline) classifyUtterance(ctx context.Context, userText string, acoustic emotion.AcousticHint, faceHint vision.Hint) emotion.UtteranceInsight {
	if p.chat == nil {
		return emotion.FallbackInsight(acoustic, faceHint)
	}
	emoSvc := p.chat.EmotionService()
	if emoSvc == nil {
		return emotion.FallbackInsight(acoustic, faceHint)
	}
	if p.vision != nil && p.vision.ContextualPlannerEnabled() && emoSvc.ClassifyEnabled() {
		return emoSvc.ClassifyUtterance(ctx, userText, acoustic, faceHint)
	}
	return emotion.FallbackInsight(acoustic, faceHint)
}

// finalizeParallelVision 兼容旧调用：仅返回 visual Hint。
func (p *Pipeline) finalizeParallelVision(ctx context.Context, sess *Session, userText string, acoustic emotion.AcousticHint, faceHint vision.Hint) vision.Hint {
	state := p.finalizeParallelPerception(ctx, sess, userText, acoustic, faceHint)
	if state == nil {
		return vision.EmptyHint()
	}
	return state.Visual
}

// maybeEarlyPerceptionAnim V3b：声学/视觉先到 → FSM 动画先行。
func (p *Pipeline) maybeEarlyPerceptionAnim(ctx context.Context, sess *Session, acoustic emotion.AcousticHint, visual vision.Hint) {
	if p.vision == nil || !p.vision.EarlyAnimationEnabled() || p.chat == nil {
		return
	}
	minAcoustic := 0.65
	minVisual := p.vision.EarlyAnimationMinConf()
	if p.vision.MinVisualConf() > minVisual {
		minVisual = p.vision.MinVisualConf()
	}
	hint := emotion.BuildEarlyHint(acoustic, visual, minAcoustic, minVisual)
	if !emotion.ShouldEarlyAnimate(hint, minVisual) {
		return
	}
	perception := agent.PerceptionResultFromHint(hint)
	p.chat.ApplyPerceptionFSM(ctx, sess.UserID, perception, true)
	log.Printf("[realtime][perception_anim] early mood=%s empathy=%v intent=%s session=%s",
		hint.UserMood, hint.NeedsEmpathy, hint.Intent, sess.ID)
}

// awaitOwnerFaceHint 等待 prefetch 或单次 VL（避免重复调用 DashScope）。
func (p *Pipeline) awaitOwnerFaceHint(ctx context.Context, sess *Session) vision.Hint {
	if p.vision == nil || !p.autoVisionOnVoiceTurn() || !sess.HasVisionFrame() {
		return vision.EmptyHint()
	}
	if sess.visualDone() {
		h := sess.VisualHint()
		log.Printf("[realtime][vision] prefetch_hit session=%s focus=%s", sess.ID, h.Focus)
		return h
	}
	timeout := p.vision.WaitTimeout()
	if waited := sess.WaitForVisualHint(ctx, timeout); waited.IsUsable() || waited.Focus != vision.FocusSkip {
		log.Printf("[realtime][vision] prefetch_hit session=%s focus=%s after_wait", sess.ID, waited.Focus)
		return waited
	}
	jpeg := sess.TurnVisionJPEG()
	if len(jpeg) == 0 {
		return vision.EmptyHint()
	}
	hint := p.vision.DescribeOwnerFace(ctx, jpeg, sess.ID)
	sess.SetVisualHint(hint)
	return hint
}

// PrefetchAcoustic 在 audio_end 时与 ASR Finish 并行跑声学（流式路径）。
func (p *Pipeline) PrefetchAcoustic(ctx context.Context, sess *Session, pcm []byte) {
	if sess.acousticDone() || p.acoustic == nil || !p.acoustic.Enabled() || len(pcm) == 0 {
		return
	}
	go func() {
		hint, err := p.acoustic.Recognize(ctx, pcm, p.asrSampleRate)
		if err != nil {
			log.Printf("[realtime] acoustic prefetch error session=%s: %v", sess.ID, err)
			return
		}
		sess.SetAcousticHint(hint)
		log.Printf("[realtime] acoustic prefetch session=%s mood=%s conf=%.2f", sess.ID, hint.Mood, hint.Confidence)
	}()
}

// resolveVisionForTurn 在已有 faceHint + acoustic 后 refine（finalizeParallelVision 内早推动画）。
func (p *Pipeline) resolveVisionForTurn(ctx context.Context, sess *Session, userText string, acoustic emotion.AcousticHint, faceHint vision.Hint) vision.Hint {
	if p.vision == nil || !p.autoVisionOnVoiceTurn() {
		return faceHint
	}
	perceiveStart := time.Now()
	hint := p.finalizeParallelVision(ctx, sess, userText, acoustic, faceHint)
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkVisionFinal()
		lat.MarkPerceiveParallel()
	}
	log.Printf("[realtime][vision] resolve session=%s text=%q focus=%s elapsed_ms=%d",
		sess.ID, truncateVisionText(userText, 40), hint.Focus, time.Since(perceiveStart).Milliseconds())
	if hint.Focus != vision.FocusSkip || !hint.Skipped {
		sess.SetVisualHint(hint)
	}
	sess.ClearVisionFrame()
	return hint
}

// perceiveVoiceTurn 流式路径：acoustic ∥ owner_face → V3c 融合。
func (p *Pipeline) perceiveVoiceTurn(ctx context.Context, sess *Session, userText string) *emotion.PerceptionState {
	log.Printf("[perception] parallel_start session=%s has_vision=%v", sess.ID, sess.HasVisionFrame())
	var (
		wg           sync.WaitGroup
		acousticHint emotion.AcousticHint
		faceHint     vision.Hint
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		acousticHint = p.ensureAcousticHint(ctx, sess)
	}()
	if p.autoVisionOnVoiceTurn() && sess.HasVisionFrame() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			faceHint = p.awaitOwnerFaceHint(ctx, sess)
		}()
	}
	wg.Wait()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkVisionFinal()
		lat.MarkPerceiveParallel()
	}
	state := p.finalizeParallelPerception(ctx, sess, userText, acousticHint, faceHint)
	if state != nil {
		sess.SetVisualHint(state.Visual)
	}
	sess.ClearVisionFrame()
	log.Printf("[perception] parallel_done session=%s text=%q final_focus=%s mood=%s",
		sess.ID, truncateVisionText(userText, 40), state.Visual.Focus, state.Hint.UserMood)
	return state
}

func truncateVisionText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func (p *Pipeline) autoVisionOnVoiceTurn() bool {
	return p.vision != nil && p.vision.Enabled()
}

// onTranscriptWithMode: LLM streams tokens; voice turns pipe sentences to TTS as they complete.
// prePerception 非空时跳过 perceiveVoiceTurn（OnSpeechEnd 已感知完毕，V3c）。
func (p *Pipeline) onTranscriptWithMode(ctx context.Context, sess *Session, text string, send Sender, withVoice bool, prePerception *emotion.PerceptionState) {
	var perception *emotion.PerceptionState
	if withVoice {
		if prePerception != nil {
			perception = prePerception
		} else {
			perception = p.perceiveVoiceTurn(ctx, sess, text)
		}
	}
	turnStarted := false
	defer func() {
		if turnStarted {
			p.completeTurn(sess, send)
		}
	}()

	// Always push ASR text to client first so chat/partial UI can show what was heard,
	// even if noise gate or response gate later dismisses the turn.
	if withVoice && strings.TrimSpace(text) != "" {
		_ = send.Send(MsgASRFinal, ASRText{Text: text})
	}

	// Noise gate: silently dismiss false-trigger ASR results (voice turns only)
	// before they reach the LLM.
	if withVoice && p.isNoiseTranscript(text) {
		log.Printf("[realtime] asr noise dismiss session=%s text=%q audio_bytes=%d", sess.ID, text, sess.TurnAudioBytes())
		p.abortTurnSilent(sess, send)
		return
	}

	// Response gate: decide whether this utterance needs a reply at all
	// (self-talk, background conversation, etc. are silently ignored).
	if withVoice && p.gate != nil {
		petName := ""
		if p.chat != nil {
			if pet, err := p.chat.GetPetByUser(ctx, sess.UserID); err == nil && pet != nil {
				petName = pet.Name
			}
		}
		if ok, reason := p.gate.Decide(ctx, text, petName); !ok {
			log.Printf("[realtime] gate dismiss session=%s text=%q reason=%s", sess.ID, text, reason)
			p.abortTurnSilent(sess, send)
			return
		}
	}

	if text == "" {
		if !withVoice {
			turnStarted = true
			_ = send.Send(MsgLLMDone, LLMDone{Text: "你好像还没输入内容？"})
			return
		}
		log.Printf("[realtime] asr empty silent dismiss session=%s audio_bytes=%d", sess.ID, sess.TurnAudioBytes())
		p.abortTurnSilent(sess, send)
		return
	}

	sess.SetState(StateThinking)
	send.SendAnimation(StateThinking)
	turnStarted = true

	if ok := p.streamLLMAndVoice(ctx, sess, send, text, withVoice, perception); !ok {
		turnStarted = false
	}
}

type segmentSynthResult struct {
	chunks [][]byte
	err    error
}

func (p *Pipeline) synthSegmentBufferedWithSynth(ctx context.Context, synth TTSSynthesizer, seg ttsSegment) segmentSynthResult {
	if synth == nil {
		synth = p.tts
	}
	if seg.text == "" {
		return segmentSynthResult{}
	}
	opts := seg.opts
	if opts.Rate == 0 && opts.Pitch == 0 && opts.Volume == 0 {
		opts = dashscope.DefaultSynthOptions()
	}
	var chunks [][]byte
	err := synth.Synthesize(ctx, seg.text, opts, func(audio []byte) {
		if len(audio) == 0 {
			return
		}
		chunks = append(chunks, append([]byte(nil), audio...))
	})
	return segmentSynthResult{chunks: chunks, err: err}
}

func (p *Pipeline) asyncSynthSegmentWithSynth(ctx context.Context, synth TTSSynthesizer, seg ttsSegment) <-chan segmentSynthResult {
	ch := make(chan segmentSynthResult, 1)
	go func() {
		ch <- p.synthSegmentBufferedWithSynth(ctx, synth, seg)
	}()
	return ch
}

// runPrefetchSegmentTTS synthesizes segments with one-ahead prefetch to hide inter-sentence gaps.
func (p *Pipeline) runPrefetchSegmentTTS(ctx context.Context, synth TTSSynthesizer, segCh <-chan ttsSegment, onChunk func([]byte), onSegmentDone func()) error {
	var ttsErr error

	var ahead <-chan segmentSynthResult

	flushSegment := func(res segmentSynthResult) {
		res = p.playSegmentResult(res, onChunk)
		if res.err != nil && ttsErr == nil {
			ttsErr = res.err
		}
		if onSegmentDone != nil && len(res.chunks) > 0 {
			onSegmentDone()
		}
	}

	for {
		var seg ttsSegment
		var ok bool

		if ahead != nil {
			flushSegment(<-ahead)
			ahead = nil

			select {
			case seg, ok = <-segCh:
				if !ok {
					return ttsErr
				}
				ahead = p.asyncSynthSegmentWithSynth(ctx, synth, seg)
			default:
			}
			continue
		}

		seg, ok = <-segCh
		if !ok {
			return ttsErr
		}

		cur := p.asyncSynthSegmentWithSynth(ctx, synth, seg)

		select {
		case nextSeg, nextOK := <-segCh:
			if nextOK {
				ahead = p.asyncSynthSegmentWithSynth(ctx, synth, nextSeg)
			} else {
				flushSegment(<-cur)
				return ttsErr
			}
		default:
		}

		flushSegment(<-cur)
	}
}

func (p *Pipeline) playSegmentResult(res segmentSynthResult, onChunk func([]byte)) segmentSynthResult {
	for _, chunk := range res.chunks {
		onChunk(chunk)
	}
	return res
}

// streamLLMAndVoice runs LLM token streaming and pipes sentence chunks to TTS asynchronously.
func (p *Pipeline) streamLLMAndVoice(ctx context.Context, sess *Session, send Sender, userText string, withVoice bool, perception *emotion.PerceptionState) bool {
	var tokenBuf strings.Builder
	speaking := false
	audioChunks := 0
	var ttsErr error
	var ttsErrMu sync.Mutex
	lat := sess.TurnLatency()
	var llmTokenMu sync.Mutex
	llmTokenSeen := false
	sentenceFlushed := false
	fillerPlayed := false
	isFirstSegment := true

	ttsBundle := p.getTTSForSession(ctx, sess)
	ttsSynth := ttsBundle.synth
	ttsFormat := ttsBundle.format
	if ttsSynth == nil {
		ttsSynth = p.tts
		ttsFormat = p.ttsFormat
	}
	defaultMood, turnEmotion := buildTurnMoodFromPerception(perception, userText)
	moodTracker := text.NewMoodTrackerWithDefault(defaultMood)
	log.Printf("[realtime][mood] default=%s empathy=%v intent=%s user_mood=%s",
		defaultMood, turnEmotion.NeedsEmpathy, turnEmotion.Intent, turnEmotion.UserMood)
	replyMoodAnimSent := false

	var opusBridge *OpusBridge
	var streamStartSent bool
	useOpus := ttsFormat == "pcm"
	if useOpus {
		var err error
		opusBridge, err = NewOpusBridge(p.cfg.TTS.SampleRate, p.cfg.TTS.Opus.Bitrate)
		if err != nil {
			log.Printf("[realtime] opus bridge failed, fallback to %s: %v", ttsFormat, err)
			useOpus = false
		}
	} else if sess.PreferMP3() || ttsFormat == "mp3" {
		log.Printf("[realtime] tts transport=%s session=%s", ttsFormat, sess.ID)
	}

	onAudio := func(audio []byte) {
		if len(audio) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		audioChunks++
		if lat != nil {
			lat.MarkTTSFirstByte()
		}
		if !speaking {
			speaking = true
			sess.SetState(StateSpeaking)
			send.SendAnimation(StateSpeaking)
			log.Printf("[realtime] tts first audio session=%s", sess.ID)
		}
		if opusBridge != nil {
			if !streamStartSent {
				streamStartSent = true
				opusBridge.SendStreamStart(send)
			}
			frames, err := opusBridge.EncodeChunk(audio)
			if err != nil {
				log.Printf("[realtime] opus encode error: %v", err)
				return
			}
			for _, frame := range frames {
				seq := sess.NextTTSSeq()
				_ = send.SendTTSAudioBinary(frame, "opus", seq)
			}
		} else {
			if ttsFormat == "pcm" {
				log.Printf("[realtime] skip pcm chunk: opus bridge unavailable session=%s", sess.ID)
				return
			}
			seq := sess.NextTTSSeq()
			_ = send.SendTTSAudioBinary(audio, ttsFormat, seq)
		}
	}

	var segCh chan ttsSegment
	var ttsDone chan struct{}
	if withVoice && ttsSynth != nil {
		segCh = make(chan ttsSegment, 16)
		ttsDone = make(chan struct{})
		go func() {
			defer close(ttsDone)
			onSegmentDone := func() {
				_ = send.Send(MsgTTSSegmentDone, map[string]any{})
			}
			err := p.runPrefetchSegmentTTS(ctx, ttsSynth, segCh, onAudio, onSegmentDone)
			if opusBridge != nil {
				if frames, errFl := opusBridge.Flush(); errFl == nil {
					for _, frame := range frames {
						seq := sess.NextTTSSeq()
						_ = send.SendTTSAudioBinary(frame, "opus", seq)
					}
				}
			}
			if err != nil {
				ttsErrMu.Lock()
				if ttsErr == nil {
					ttsErr = err
				}
				ttsErrMu.Unlock()
				log.Printf("[realtime] tts segment error session=%s: %v", sess.ID, err)
			}
		}()
	}

	enqueueSeg := func(raw string, markSentence bool) {
		raw = strings.TrimSpace(raw)
		if raw == "" || segCh == nil {
			return
		}
		tone := moodTracker.Process(raw)
		if tone.Text == "" {
			return
		}
		if !replyMoodAnimSent && p.chat != nil {
			if anim := animationForReplyMood(tone.Mood); anim != "" {
				replyMoodAnimSent = true
				p.chat.BroadcastReplyMood(ctx, sess.UserID, tone.Mood)
				log.Printf("[realtime][mood_anim] session=%s mood=%s anim=%s", sess.ID, tone.Mood, anim)
			}
		}
		opts := ProsodyForMood(tone.Mood, ttsBundle.baseline).ToSynthOptions()
		if markSentence && lat != nil && !sentenceFlushed {
			sentenceFlushed = true
			lat.MarkLLMFirstSentence()
		}
		select {
		case segCh <- ttsSegment{text: tone.Text, opts: opts}:
		case <-ctx.Done():
		}
	}

	flushBuffer := func() {
		for {
			seg := takeFlushSegmentEx(&tokenBuf, p.cfg.Pipeline, isFirstSegment)
			if seg == "" {
				break
			}
			isFirstSegment = false
			enqueueSeg(seg, true)
		}
	}

	// Thinking filler: play a short phrase if LLM is slow to respond.
	if withVoice && p.tts != nil && p.cfg.ThinkingFiller.Enabled {
		go func() {
			threshold := time.Duration(p.cfg.ThinkingFiller.ThresholdMS) * time.Millisecond
			timer := time.NewTimer(threshold)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				llmTokenMu.Lock()
				seen := llmTokenSeen
				llmTokenMu.Unlock()
				if seen || fillerPlayed {
					return
				}
				phrases := p.cfg.ThinkingFiller.Phrases
				if len(phrases) == 0 {
					return
				}
				phrase := phrases[rand.Intn(len(phrases))]
				fillerPlayed = true
				if lat != nil {
					lat.MarkFillerPlayed()
				}
				log.Printf("[realtime] thinking filler session=%s phrase=%q", sess.ID, phrase)
				enqueueSeg(phrase, false)
			}
		}()
	}

	var uiMoodStrip text.StreamMoodStripper
	reply, err := p.chat.StreamMessageVoice(ctx, sess.UserID, userText, perceptionAcoustic(perception), perceptionVisual(perception), perception, func(token string) {
		llmTokenMu.Lock()
		if !llmTokenSeen {
			llmTokenSeen = true
			if lat != nil {
				lat.MarkLLMFirstToken()
			}
		}
		llmTokenMu.Unlock()
		if token != "" {
			ui := uiMoodStrip.Feed(token)
			if ui != "" {
				_ = send.Send(MsgLLMToken, LLMToken{Token: ui})
			}
			tokenBuf.WriteString(token)
		}
		if withVoice && p.tts != nil {
			flushBuffer()
		}
	})
	if err != nil {
		if segCh != nil {
			close(segCh)
			<-ttsDone
		}
		if p.handleCancelled(ctx, sess, send, err) {
			return false
		}
		log.Printf("[realtime] llm error session=%s: %v", sess.ID, err)
		msg := fmt.Sprintf("AI 回复失败: %v", err)
		if strings.Contains(err.Error(), "pet not found") {
			msg = "还没有宠物，请先在主页创建 Mochi"
		} else if strings.Contains(err.Error(), "load pet") {
			msg = "数据库连接不稳定，请稍后再试"
		}
		p.failTurn(ctx, sess, send, "LLM_FAILED", msg)
		return false
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		if segCh != nil {
			close(segCh)
			<-ttsDone
		}
		p.handleCancelled(ctx, sess, send, ctx.Err())
		return false
	}

	if reply == "" {
		reply = strings.TrimSpace(tokenBuf.String())
	}
	if reply == "" {
		if segCh != nil {
			close(segCh)
			<-ttsDone
		}
		log.Printf("[realtime] llm empty silent dismiss session=%s", sess.ID)
		p.abortTurnSilent(sess, send)
		return false
	}
	_ = send.Send(MsgLLMDone, LLMDone{Text: reply})
	if compliance := text.MoodTagComplianceRate(reply); reply != "" {
		log.Printf("[realtime][mood] compliance=%.0f%% tags=%d sentences=%d session=%s",
			compliance*100, text.CountMoodTags(reply), text.CountSpeakSentences(reply), sess.ID)
	}

	if withVoice && p.tts != nil && segCh != nil {
		if remainder := strings.TrimSpace(tokenBuf.String()); remainder != "" {
			enqueueSeg(remainder, true)
		}
		close(segCh)
		<-ttsDone

		ttsErrMu.Lock()
		errSnapshot := ttsErr
		ttsErrMu.Unlock()

		if audioChunks == 0 {
			if errSnapshot != nil {
				p.failTurn(ctx, sess, send, "TTS_FAILED", "语音合成失败，请稍后再试")
				return false
			}
			log.Printf("[realtime] sentence tts empty, batch fallback session=%s", sess.ID)
			if !p.speakAudio(ctx, sess, send, reply) {
				return false
			}
		} else if errSnapshot != nil {
			log.Printf("[realtime] partial tts error after %d chunks session=%s: %v", audioChunks, sess.ID, errSnapshot)
		} else {
			log.Printf("[realtime] sentence tts sent %d audio chunks session=%s", audioChunks, sess.ID)
		}
	}

	return true
}

func (p *Pipeline) speakReply(ctx context.Context, sess *Session, send Sender, reply string) {
	_ = send.Send(MsgLLMDone, LLMDone{Text: reply})

	if p.tts == nil {
		return
	}

	sess.SetState(StateSpeaking)
	send.SendAnimation(StateSpeaking)
	_ = p.speakAudio(ctx, sess, send, reply)
}

func (p *Pipeline) speakAudio(ctx context.Context, sess *Session, send Sender, reply string) bool {
	lat := sess.TurnLatency()
	var chunks int
	var opusBridge *OpusBridge
	var streamStartSent bool
	ttsBundle := p.getTTSForSession(ctx, sess)
	ttsSynth := ttsBundle.synth
	ttsFormat := ttsBundle.format
	if ttsSynth == nil {
		ttsSynth = p.tts
		ttsFormat = p.ttsFormat
	}

	useOpus := ttsFormat == "pcm"
	if useOpus {
		var err error
		opusBridge, err = NewOpusBridge(p.cfg.TTS.SampleRate, p.cfg.TTS.Opus.Bitrate)
		if err != nil {
			log.Printf("[realtime] opus bridge failed in speakAudio, fallback to %s: %v", ttsFormat, err)
			useOpus = false
		}
	}

	firstMood := text.ParseToneSegment(reply).Mood
	if firstMood == "" {
		firstMood = text.MoodCalm
	}
	tone := text.NewMoodTrackerWithDefault(firstMood).Process(reply)
	speakText := tone.Text
	if speakText == "" {
		speakText = text.StripMoodTags(reply)
	}
	opts := ProsodyForMood(tone.Mood, ttsBundle.baseline).ToSynthOptions()

	err := ttsSynth.Synthesize(ctx, speakText, opts, func(audio []byte) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if len(audio) == 0 {
			return
		}
		chunks++
		if lat != nil {
			lat.MarkTTSFirstByte()
		}
		if opusBridge != nil {
			if !streamStartSent {
				streamStartSent = true
				opusBridge.SendStreamStart(send)
			}
			frames, err := opusBridge.EncodeChunk(audio)
			if err == nil {
				for _, frame := range frames {
					seq := sess.NextTTSSeq()
					_ = send.SendTTSAudioBinary(frame, "opus", seq)
				}
			}
		} else {
			if ttsFormat == "pcm" {
				log.Printf("[realtime] skip pcm chunk in speakAudio: opus bridge unavailable session=%s", sess.ID)
				return
			}
			seq := sess.NextTTSSeq()
			_ = send.SendTTSAudioBinary(audio, ttsFormat, seq)
		}
	})

	if opusBridge != nil && useOpus {
		if frames, errFl := opusBridge.Flush(); errFl == nil {
			for _, frame := range frames {
				seq := sess.NextTTSSeq()
				_ = send.SendTTSAudioBinary(frame, "opus", seq)
			}
		}
	}

	if err != nil {
		if p.handleCancelled(ctx, sess, send, err) {
			return false
		}
		log.Printf("[realtime] tts synthesize error session=%s: %v", sess.ID, err)
		p.failTurn(ctx, sess, send, "TTS_FAILED", "语音播放失败，请稍后再试")
		return false
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		p.handleCancelled(ctx, sess, send, ctx.Err())
		return false
	}

	if chunks == 0 {
		log.Printf("[realtime] tts returned no audio session=%s", sess.ID)
		p.failTurn(ctx, sess, send, "TTS_FAILED", "语音播放失败，请稍后再试")
		return false
	}
	log.Printf("[realtime] tts sent %d chunks session=%s", chunks, sess.ID)
	return true
}

// completeTurn always sends tts_done so the client can resume listening.
func (p *Pipeline) completeTurn(sess *Session, send Sender) {
	if lat := sess.TurnLatency(); lat != nil {
		_ = send.Send(MsgTurnMetrics, lat.ToMetrics())
		lat.LogSummary(sess.ID)
	}
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

func (p *Pipeline) handleCancelled(ctx context.Context, sess *Session, send Sender, err error) bool {
	if !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	log.Printf("[realtime] pipeline interrupted session=%s", sess.ID)
	sess.ClearTurnLatency()
	_ = send.Send(MsgInterrupted, map[string]any{})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
	return true
}

func (p *Pipeline) setListening(sess *Session, send Sender) {
	sess.SetState(StateListening)
	send.SendAnimation(StateListening)
}

func (p *Pipeline) abortTurnSilent(sess *Session, send Sender) {
	if lat := sess.TurnLatency(); lat != nil {
		lat.LogSummary(sess.ID)
		sess.ClearTurnLatency()
	}
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

func (p *Pipeline) failTurn(_ context.Context, sess *Session, send Sender, code, message string) {
	if lat := sess.TurnLatency(); lat != nil {
		lat.LogSummary(sess.ID)
		_ = send.Send(MsgTurnMetrics, lat.ToMetrics())
		sess.ClearTurnLatency()
	}
	_ = send.Send(MsgError, ErrorData{Code: code, Message: message})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

// isNoiseTranscript reports whether an ASR transcript is a likely false
// trigger: empty or consisting solely of modal filler runes from config.
func (p *Pipeline) isNoiseTranscript(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	rs := make([]rune, 0, len(t))
	for _, r := range t {
		if unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r) {
			continue
		}
		rs = append(rs, r)
	}
	if len(rs) == 0 {
		return true
	}
	allFiller := true
	for _, r := range rs {
		if !p.noiseFillers[r] {
			allFiller = false
			break
		}
	}
	return allFiller
}

func (p *Pipeline) Interrupt(sess *Session, send Sender) {
	sess.CancelPipeline()
	sess.ClearTurnLatency()
	_ = send.Send(MsgInterrupted, map[string]any{})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

func buildTurnMoodFromPerception(state *emotion.PerceptionState, userText string) (text.MoodTag, emotion.Hint) {
	if state != nil {
		return text.InferDefaultMood(state.Hint.UserMood, state.Hint.Intent, state.Hint.NeedsEmpathy), state.Hint
	}
	return buildTurnMoodContext(userText, emotion.EmptyAcousticHint(), vision.EmptyHint())
}

func perceptionAcoustic(state *emotion.PerceptionState) emotion.AcousticHint {
	if state == nil {
		return emotion.EmptyAcousticHint()
	}
	return state.Acoustic
}

func perceptionVisual(state *emotion.PerceptionState) vision.Hint {
	if state == nil {
		return vision.EmptyHint()
	}
	return state.Visual
}

func buildTurnMoodContext(userText string, acoustic emotion.AcousticHint, visual vision.Hint) (text.MoodTag, emotion.Hint) {
	quick := emotion.QuickDetect(userText)
	merged := emotion.MergeAcousticHint(emotion.Hint{}, quick, acoustic, 0.65)
	merged = emotion.MergeVisualHint(merged, visual, 0.6)
	return text.InferDefaultMood(merged.UserMood, merged.Intent, merged.NeedsEmpathy), merged
}

func animationForReplyMood(m text.MoodTag) string {
	switch m {
	case text.MoodSad, text.MoodGentle:
		return "sad"
	case text.MoodWorried:
		return "worried"
	case text.MoodExcited, text.MoodPlayful:
		return "happy"
	default:
		return ""
	}
}
