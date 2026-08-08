package voice

import (
	"context"
	"fmt"
	"time"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/realtime"
)

type Service struct {
	asr realtime.ASRRecognizer
	tts realtime.TTSSynthesizer
}

func NewService(cfg *config.Config) *Service {
	rt := cfg.Realtime
	wsURL := rt.XASR.WSURL
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:8766"
	}
	baseURL := rt.XTTS.BaseURL
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8767"
	}
	timeout := time.Duration(rt.XTTS.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Service{
		asr: realtime.NewXasrASR(wsURL, rt.ASR.SampleRate),
		tts: realtime.NewXttsSynth(baseURL, rt.XTTS.Speed, timeout),
	}
}

func (s *Service) Recognize(ctx context.Context, audio []byte, format string) (string, error) {
	if s.asr == nil {
		return "", fmt.Errorf("ASR not configured")
	}
	pcm, err := audioToPCM(audio, format)
	if err != nil {
		return "", err
	}
	return s.asr.Recognize(ctx, pcm, nil)
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if s.tts == nil {
		return nil, "", fmt.Errorf("TTS not configured")
	}
	var out []byte
	err := s.tts.Synthesize(ctx, text, realtime.DefaultSynthOptions(), func(chunk []byte) {
		if len(chunk) > 0 {
			out = append(out, chunk...)
		}
	})
	if err != nil {
		return nil, "", err
	}
	if len(out) == 0 {
		return nil, "", fmt.Errorf("TTS returned no audio")
	}
	return out, "wav", nil
}
