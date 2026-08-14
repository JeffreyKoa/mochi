package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func boolPtrTest(b bool) *bool { return &b }

func TestBuildASRRecognizer_disabled(t *testing.T) {
	f := false
	cfg := &config.Config{
		Modules: config.ModulesConfig{
			ASR: config.ModuleEntryConfig{Enabled: &f, Provider: "local"},
		},
	}
	if BuildASRRecognizer(cfg, cfg.Realtime) != nil {
		t.Fatal("disabled asr should be nil")
	}
}

func TestBuildASRRecognizer_remote(t *testing.T) {
	cfg := &config.Config{
		Modules: config.ModulesConfig{
			ASR: config.ModuleEntryConfig{Enabled: boolPtrTest(true), Provider: "remote"},
		},
		AI: config.AIConfig{
			APIBase:  "https://api.openai.com/v1",
			APIKey:   "sk-test",
			ASRModel: "whisper-1",
		},
	}
	if cfg.AI.ASRModel == "" {
		cfg.AI.ASRModel = "whisper-1"
	}
	r := BuildASRRecognizer(cfg, cfg.Realtime)
	if r == nil {
		t.Fatal("remote asr expected")
	}
}
