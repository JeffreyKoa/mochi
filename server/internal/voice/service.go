package voice

import (
	"context"
	"fmt"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/tencent"
)

type Service struct {
	asr *tencent.ASR
	tts *tencent.TTS
}

func NewService(cfg *config.Config) *Service {
	asrCfg := cfg.ASR
	ttsCfg := cfg.TTSConfig()

	client := tencent.NewClient(asrCfg.SecretID, asrCfg.SecretKey, cfg.ASRRegion())
	return &Service{
		asr: tencent.NewASR(client),
		tts: tencent.NewTTS(client, ttsCfg.VoiceType),
	}
}

func (s *Service) Recognize(ctx context.Context, audio []byte, format string) (string, error) {
	if s.asr == nil {
		return "", fmt.Errorf("ASR not configured in config.yaml")
	}
	return s.asr.SentenceRecognition(ctx, audio, format)
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if s.tts == nil {
		return nil, "", fmt.Errorf("TTS not configured in config.yaml")
	}
	return s.tts.TextToVoice(ctx, text)
}
