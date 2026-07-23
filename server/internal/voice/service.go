package voice

import (
	"context"
	"fmt"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/dashscope"
)

type Service struct {
	asr *dashscope.ASRClient
	tts *dashscope.TTSClient
}

func NewService(cfg *config.Config) *Service {
	key := cfg.AI.APIKey
	rt := cfg.Realtime
	if key == "" {
		return &Service{}
	}
	return &Service{
		asr: dashscope.NewASRClient(key, rt.ASR.Model, rt.ASR.SampleRate, dashscope.EndpointConfig{
			WSURL: rt.Dashscope.ASRWSURL,
		}),
		tts: dashscope.NewTTSClient(key, rt.TTS.Model, rt.TTS.Voice, rt.TTS.SampleRate, dashscope.EndpointConfig{
			WSURL:       rt.Dashscope.WSURL,
			WorkspaceID: rt.Dashscope.WorkspaceID,
			Region:      rt.Dashscope.Region,
		}),
	}
}

func (s *Service) Recognize(ctx context.Context, audio []byte, format string) (string, error) {
	if s.asr == nil {
		return "", fmt.Errorf("ASR not configured: set ai.api_key in config.yaml")
	}
	pcm, err := audioToPCM(audio, format)
	if err != nil {
		return "", err
	}
	return s.asr.Recognize(ctx, pcm, nil)
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if s.tts == nil {
		return nil, "", fmt.Errorf("TTS not configured: set ai.api_key in config.yaml")
	}
	var out []byte
	err := s.tts.Synthesize(ctx, text, func(chunk []byte) {
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
	return out, s.tts.AudioFormat(), nil
}
