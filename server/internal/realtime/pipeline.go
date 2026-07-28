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
}

func NewPipeline(chatSvc *chat.Service, cfg config.RealtimeConfig, appCfg *config.Config) *Pipeline {
	apiKey := appCfg.AI.APIKey
	ep := dashscope.EndpointConfig{
		WSURL:       cfg.Dashscope.WSURL,
		WorkspaceID: cfg.Dashscope.WorkspaceID,
		Region:      cfg.Dashscope.Region,
	}
	p := &Pipeline{chat: chatSvc, cfg: cfg, ttsFormat: "mp3", apiKey: apiKey, ep: ep}
	asrEp := dashscope.EndpointConfig{WSURL: cfg.Dashscope.ASRWSURL}
	if cfg.ASR.Provider == "dashscope" && apiKey != "" {
		p.asr = newDashscopeASR(dashscope.NewASRClient(apiKey, cfg.ASR.Model, cfg.ASR.SampleRate, asrEp))
	}

	p.tts, p.ttsFormat = buildTTSSynth(cfg, apiKey, ep, ttsPreferMP3(cfg, false))
	p.gate = NewResponseGate(cfg.Gate, apiKey, appCfg.AI.APIBase)
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

func (p *Pipeline) getTTSForSession(ctx context.Context, sess *Session) (TTSSynthesizer, string) {
	preferMP3 := ttsPreferMP3(p.cfg, sess.PreferMP3())
	if p.tts == nil {
		return nil, "mp3"
	}
	if p.chat == nil {
		return p.tts, p.ttsFormat
	}
	pet, err := p.chat.GetPetByUser(ctx, sess.UserID)
	if err != nil || pet == nil {
		return p.tts, p.ttsFormat
	}

	profile := ResolveVoice(pet.Gender, pet.LifeStage, string(pet.PersonalityJSON))
	cfg := p.cfg
	if profile.DashscopeVoice != "" {
		cfg.TTS.Voice = profile.DashscopeVoice
	}

	synth, fmtStr := buildTTSSynth(cfg, p.apiKey, p.ep, preferMP3)
	if synth != nil {
		return synth, fmtStr
	}
	return p.tts, p.ttsFormat
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
		if err := p.tts.Synthesize(warmCtx, "嗯", func([]byte) {}); err != nil {
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
	sess.SetTurnAudioBytes(len(audio))

	if p.asr == nil {
		p.failTurn(ctx, sess, send, "ASR_NOT_CONFIGURED", "ASR 未配置，请在 config.yaml 设置 ai.api_key")
		return
	}

	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()

	var lastPartial string
	text, err := p.asr.Recognize(pipeCtx, audio, func(partial string, _ bool) {
		if partial == lastPartial {
			return
		}
		lastPartial = partial
		_ = send.Send(MsgASRPartial, ASRText{Text: partial})
	})
	if err != nil {
		if p.handleCancelled(pipeCtx, sess, send, err) {
			return
		}
		log.Printf("[realtime] asr error session=%s: %v", sess.ID, err)
		p.failTurn(ctx, sess, send, "ASR_FAILED", fmt.Sprintf("语音识别失败: %v", err))
		return
	}

	if text == "" {
		text = lastPartial
	}
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true)
}

func (p *Pipeline) OnTranscript(ctx context.Context, sess *Session, text string, send Sender) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true)
}

func (p *Pipeline) OnTextInput(ctx context.Context, sess *Session, text string, send Sender, withVoice bool) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	if lat := sess.TurnLatency(); lat != nil {
		lat.MarkASRFinal()
	}
	p.onTranscriptWithMode(pipeCtx, sess, text, send, withVoice)
}

// onTranscriptWithMode: LLM streams tokens; voice turns pipe sentences to TTS as they complete.
func (p *Pipeline) onTranscriptWithMode(ctx context.Context, sess *Session, text string, send Sender, withVoice bool) {
	turnStarted := false
	defer func() {
		if turnStarted {
			p.completeTurn(sess, send)
		}
	}()

	// Noise gate: silently dismiss false-trigger ASR results (voice turns only)
	// before they reach the chat history or the LLM.
	if withVoice && isNoiseTranscript(text) {
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

	_ = send.Send(MsgASRFinal, ASRText{Text: text})

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

	if ok := p.streamLLMAndVoice(ctx, sess, send, text, withVoice); !ok {
		turnStarted = false
	}
}

type segmentSynthResult struct {
	chunks [][]byte
	err    error
}

func (p *Pipeline) synthSegmentBufferedWithSynth(ctx context.Context, synth TTSSynthesizer, text string) segmentSynthResult {
	if synth == nil {
		synth = p.tts
	}
	var chunks [][]byte
	err := synth.Synthesize(ctx, text, func(audio []byte) {
		if len(audio) == 0 {
			return
		}
		chunks = append(chunks, append([]byte(nil), audio...))
	})
	return segmentSynthResult{chunks: chunks, err: err}
}

func (p *Pipeline) asyncSynthSegmentWithSynth(ctx context.Context, synth TTSSynthesizer, text string) <-chan segmentSynthResult {
	ch := make(chan segmentSynthResult, 1)
	go func() {
		ch <- p.synthSegmentBufferedWithSynth(ctx, synth, text)
	}()
	return ch
}

// runPrefetchSegmentTTS synthesizes segments with one-ahead prefetch to hide inter-sentence gaps.
func (p *Pipeline) runPrefetchSegmentTTS(ctx context.Context, synth TTSSynthesizer, segCh <-chan string, onChunk func([]byte), onSegmentDone func()) error {
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
		var seg string
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
func (p *Pipeline) streamLLMAndVoice(ctx context.Context, sess *Session, send Sender, userText string, withVoice bool) bool {
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

	ttsSynth, ttsFormat := p.getTTSForSession(ctx, sess)
	if ttsSynth == nil {
		ttsSynth = p.tts
		ttsFormat = p.ttsFormat
	}

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

	var segCh chan string
	var ttsDone chan struct{}
	if withVoice && ttsSynth != nil {
		segCh = make(chan string, 16)
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

	enqueueSeg := func(text string, markSentence bool) {
		text = strings.TrimSpace(text)
		if text == "" || segCh == nil {
			return
		}
		if markSentence && lat != nil && !sentenceFlushed {
			sentenceFlushed = true
			lat.MarkLLMFirstSentence()
		}
		select {
		case segCh <- text:
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

	reply, err := p.chat.StreamMessage(ctx, sess.UserID, userText, func(token string) {
		llmTokenMu.Lock()
		if !llmTokenSeen {
			llmTokenSeen = true
			if lat != nil {
				lat.MarkLLMFirstToken()
			}
		}
		llmTokenMu.Unlock()
		_ = send.Send(MsgLLMToken, LLMToken{Token: token})
		tokenBuf.WriteString(token)
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
	ttsSynth, ttsFormat := p.getTTSForSession(ctx, sess)
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

	err := ttsSynth.Synthesize(ctx, reply, func(audio []byte) {
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

// fillerRunes are standalone filler/exclamation characters. An ASR transcript
// consisting only of these is treated as noise (coughs, sighs, background talk).
var fillerRunes = map[rune]bool{
	'嗯': true, '啊': true, '呃': true, '哦': true, '唉': true, '哎': true,
	'哼': true, '咳': true, '额': true, '噢': true, '喔': true, '呀': true,
	'哈': true, '嘿': true, '唔': true, '喂': true, '呐': true, '嗐': true,
}

// isNoiseTranscript reports whether an ASR transcript is a likely false
// trigger: empty, shorter than 2 meaningful characters, or filler-only.
func isNoiseTranscript(text string) bool {
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
	if len(rs) < 2 {
		return true
	}
	for _, r := range rs {
		if !fillerRunes[r] {
			return false
		}
	}
	return true
}

func (p *Pipeline) Interrupt(sess *Session, send Sender) {
	sess.CancelPipeline()
	sess.ClearTurnLatency()
	_ = send.Send(MsgInterrupted, map[string]any{})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}
