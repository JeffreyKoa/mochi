package realtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/dashscope"
	"github.com/mochi-ai/server/pkg/tencent"
)

// Pipeline orchestrates ASR → LLM → TTS (turn-based, half-duplex).
type Pipeline struct {
	chat      *chat.Service
	cfg       config.RealtimeConfig
	asr       ASRRecognizer
	tts       TTSSynthesizer
	ttsFormat string
}

func NewPipeline(chatSvc *chat.Service, cfg config.RealtimeConfig, appCfg *config.Config) *Pipeline {
	p := &Pipeline{chat: chatSvc, cfg: cfg, ttsFormat: "mp3"}
	apiKey := appCfg.AI.APIKey
	if cfg.ASR.Provider == "dashscope" && apiKey != "" {
		p.asr = newDashscopeASR(dashscope.NewASRClient(apiKey, cfg.ASR.Model, cfg.ASR.SampleRate))
	}

	var primary TTSSynthesizer
	var backup TTSSynthesizer
	if cfg.TTS.Provider == "dashscope" && apiKey != "" {
		client := dashscope.NewTTSClient(apiKey, cfg.TTS.Model, cfg.TTS.Voice, cfg.TTS.SampleRate)
		primary = newDashscopeTTSSynth(client)
		p.ttsFormat = client.AudioFormat()
	}
	ttsCfg := appCfg.TTSConfig()
	if ttsCfg.SecretID != "" && ttsCfg.SecretKey != "" {
		tc := tencent.NewClient(ttsCfg.SecretID, ttsCfg.SecretKey, appCfg.ASRRegion())
		backup = newTencentTTSSynth(tencent.NewTTS(tc, ttsCfg.VoiceType))
	}
	switch {
	case primary != nil && backup != nil:
		p.tts = &fallbackTTSSynth{primary: primary, backup: backup, name: "dashscope"}
	case primary != nil:
		p.tts = primary
	case backup != nil:
		p.tts = backup
	}
	return p
}

func (p *Pipeline) StartASRSession(ctx context.Context, onPartial func(text string)) (ASRSession, error) {
	if p.asr == nil {
		return nil, fmt.Errorf("asr not configured")
	}
	return p.asr.StartSession(ctx, onPartial)
}

func (p *Pipeline) OnSpeechEnd(ctx context.Context, sess *Session, audio []byte, send Sender) {
	sess.SetState(StateThinking)
	send.SendAnimation(StateThinking)

	log.Printf("[realtime] session=%s user=%d audio_bytes=%d", sess.ID, sess.UserID, len(audio))

	if p.asr == nil {
		p.failTurn(ctx, sess, send, "ASR_NOT_CONFIGURED", "ASR 未配置，请在 config.yaml 设置 ai.api_key")
		return
	}

	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()

	var lastPartial string
	text, err := p.asr.Recognize(pipeCtx, audio, func(partial string) {
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
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true)
}

func (p *Pipeline) OnTranscript(ctx context.Context, sess *Session, text string, send Sender) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	p.onTranscriptWithMode(pipeCtx, sess, text, send, true)
}

func (p *Pipeline) OnTextInput(ctx context.Context, sess *Session, text string, send Sender) {
	pipeCtx := sess.BeginPipeline(ctx)
	defer sess.EndPipeline()
	p.onTranscriptWithMode(pipeCtx, sess, text, send, false)
}

// onTranscriptWithMode: LLM streams text tokens; voice turns also batch TTS after reply.
func (p *Pipeline) onTranscriptWithMode(ctx context.Context, sess *Session, text string, send Sender, withVoice bool) {
	turnStarted := false
	defer func() {
		if turnStarted {
			p.completeTurn(sess, send)
		}
	}()

	_ = send.Send(MsgASRFinal, ASRText{Text: text})

	if text == "" {
		turnStarted = true
		p.speakReply(ctx, sess, send, "没有听清楚，可以再说一次吗？")
		return
	}

	sess.SetState(StateThinking)
	send.SendAnimation(StateThinking)
	turnStarted = true

	reply, err := p.chat.StreamMessage(ctx, sess.UserID, text, func(token string) {
		_ = send.Send(MsgLLMToken, LLMToken{Token: token})
	})
	if err != nil {
		if p.handleCancelled(ctx, sess, send, err) {
			turnStarted = false
			return
		}
		log.Printf("[realtime] llm error session=%s: %v", sess.ID, err)
		msg := fmt.Sprintf("AI 回复失败: %v", err)
		if strings.Contains(err.Error(), "pet not found") {
			msg = "还没有宠物，请先在主页创建 Mochi"
		} else if strings.Contains(err.Error(), "load pet") {
			msg = "数据库连接不稳定，请稍后再试"
		}
		p.failTurn(ctx, sess, send, "LLM_FAILED", msg)
		turnStarted = false
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		p.handleCancelled(ctx, sess, send, ctx.Err())
		turnStarted = false
		return
	}

	if reply == "" {
		reply = "嗯... 让我想想~"
	}
	_ = send.Send(MsgLLMDone, LLMDone{Text: reply})

	if errors.Is(ctx.Err(), context.Canceled) {
		p.handleCancelled(ctx, sess, send, ctx.Err())
		turnStarted = false
		return
	}

	if !withVoice || p.tts == nil || reply == "" {
		return
	}

	sess.SetState(StateSpeaking)
	send.SendAnimation(StateSpeaking)

	if !p.speakAudio(ctx, sess, send, reply) {
		turnStarted = false
	}
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
	var chunks int
	if err := p.tts.Synthesize(ctx, reply, func(audio []byte) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if len(audio) == 0 {
			return
		}
		chunks++
		seq := sess.NextTTSSeq()
		_ = send.Send(MsgTTSAudio, TTSAudio{
			PCM:    base64.StdEncoding.EncodeToString(audio),
			Format: p.ttsFormat,
			Seq:    seq,
		})
	}); err != nil {
		if p.handleCancelled(ctx, sess, send, err) {
			return false
		}
		log.Printf("[realtime] tts synthesize error session=%s: %v", sess.ID, err)
		p.failTurn(ctx, sess, send, "TTS_FAILED", fmt.Sprintf("语音合成失败: %v", err))
		return false
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		p.handleCancelled(ctx, sess, send, ctx.Err())
		return false
	}

	if chunks == 0 {
		log.Printf("[realtime] tts returned no audio session=%s", sess.ID)
		p.failTurn(ctx, sess, send, "TTS_FAILED", "语音合成未返回音频")
		return false
	}
	log.Printf("[realtime] tts sent %d chunks session=%s", chunks, sess.ID)
	return true
}

// completeTurn always sends tts_done so the client can resume listening.
func (p *Pipeline) completeTurn(sess *Session, send Sender) {
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

func (p *Pipeline) handleCancelled(ctx context.Context, sess *Session, send Sender, err error) bool {
	if !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	log.Printf("[realtime] pipeline interrupted session=%s", sess.ID)
	_ = send.Send(MsgInterrupted, map[string]any{})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
	return true
}

func (p *Pipeline) setListening(sess *Session, send Sender) {
	sess.SetState(StateListening)
	send.SendAnimation(StateListening)
}

func (p *Pipeline) failTurn(_ context.Context, sess *Session, send Sender, code, message string) {
	_ = send.Send(MsgError, ErrorData{Code: code, Message: message})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}

func (p *Pipeline) Interrupt(sess *Session, send Sender) {
	sess.CancelPipeline()
	_ = send.Send(MsgInterrupted, map[string]any{})
	_ = send.Send(MsgTTSDone, map[string]any{})
	p.setListening(sess, send)
}
