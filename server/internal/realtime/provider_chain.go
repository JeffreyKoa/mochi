package realtime

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// chainASR 先本地后远程（auto fallback）。
type chainASR struct {
	primary  ASRRecognizer
	fallback ASRRecognizer
	label    string
}

func newChainASR(primary, fallback ASRRecognizer, label string) ASRRecognizer {
	if primary == nil && fallback == nil {
		return nil
	}
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &chainASR{primary: primary, fallback: fallback, label: label}
}

func (c *chainASR) Recognize(ctx context.Context, pcm []byte, onPartial ASRPartialHandler) (string, error) {
	text, err := c.primary.Recognize(ctx, pcm, onPartial)
	if err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	if c.fallback == nil {
		return text, err
	}
	log.Printf("[realtime] asr %s primary failed (%v), fallback remote", c.label, err)
	ft, fbErr := c.fallback.Recognize(ctx, pcm, onPartial)
	if fbErr == nil && strings.TrimSpace(ft) != "" {
		return ft, nil
	}
	if err != nil {
		return "", err
	}
	return "", fbErr
}

func (c *chainASR) StartSession(ctx context.Context, onPartial ASRPartialHandler) (ASRSession, error) {
	sess, err := c.primary.StartSession(ctx, onPartial)
	if err == nil {
		return sess, nil
	}
	if c.fallback == nil {
		return nil, err
	}
	log.Printf("[realtime] asr %s session primary failed (%v), no streaming fallback", c.label, err)
	return nil, fmt.Errorf("local asr unavailable: %w (use batch or configure remote)", err)
}

// chainTTS 先本地后远程（auto fallback）。
type chainTTS struct {
	primary  TTSSynthesizer
	fallback TTSSynthesizer
	label    string
}

func newChainTTS(primary, fallback TTSSynthesizer, label string) TTSSynthesizer {
	if primary == nil && fallback == nil {
		return nil
	}
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &chainTTS{primary: primary, fallback: fallback, label: label}
}

func (c *chainTTS) StartSession(ctx context.Context, opts SynthOptions, onAudio func([]byte)) (TTSSession, error) {
	sess, err := c.primary.StartSession(ctx, opts, onAudio)
	if err == nil {
		return sess, nil
	}
	if c.fallback == nil {
		return nil, err
	}
	log.Printf("[realtime] tts %s session primary failed (%v), use fallback synthesize", c.label, err)
	return c.fallback.StartSession(ctx, opts, onAudio)
}

func (c *chainTTS) Synthesize(ctx context.Context, text string, opts SynthOptions, onAudio func([]byte)) error {
	err := c.primary.Synthesize(ctx, text, opts, onAudio)
	if err == nil {
		return nil
	}
	if c.fallback == nil {
		return err
	}
	log.Printf("[realtime] tts %s primary failed (%v), fallback remote", c.label, err)
	return c.fallback.Synthesize(ctx, text, opts, onAudio)
}
