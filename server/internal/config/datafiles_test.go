package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDataFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "config")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("config dir not found")
	}

	cfg := &Config{
		Realtime: RealtimeConfig{
			Gate: RealtimeGate{},
			ASR:  RealtimeASR{},
		},
	}
	if err := cfg.loadDataFiles(dir); err != nil {
		t.Fatal(err)
	}
	if len(cfg.GateFastpath.QuestionWords) == 0 {
		t.Fatal("expected question words")
	}
	if cfg.GateSystemPrompt == "" {
		t.Fatal("expected gate system prompt")
	}
	if len(cfg.NoiseFillers) == 0 {
		t.Fatal("expected noise fillers")
	}
	if cfg.NoiseFillers['咳'] != true {
		t.Fatal("expected 咳 in noise fillers")
	}
	if cfg.NoiseFillers['好'] == true {
		t.Fatal("好 should not be noise filler")
	}
}

func TestRealtimePipelineApplyDefaults(t *testing.T) {
	p := RealtimePipeline{}
	p.applyDefaults()
	if p.TTSMinChars != 8 {
		t.Fatalf("TTSMinChars=%d", p.TTSMinChars)
	}
	if p.TTSFirstMinChars != 3 {
		t.Fatalf("TTSFirstMinChars=%d", p.TTSFirstMinChars)
	}
	if p.TTSForceFlushChars != 24 {
		t.Fatalf("TTSForceFlushChars=%d", p.TTSForceFlushChars)
	}
}
