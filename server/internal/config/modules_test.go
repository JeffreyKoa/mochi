package config

import "testing"

func TestSyncModules_legacyASRNone(t *testing.T) {
	cfg := &Config{
		Realtime: RealtimeConfig{
			ASR: RealtimeASR{Provider: "none"},
		},
	}
	cfg.applyDefaults()
	if cfg.IsASREnabled() {
		t.Fatal("asr should be disabled when provider=none")
	}
}

func TestSyncModules_modulesVisionDisabled(t *testing.T) {
	f := false
	cfg := &Config{
		Modules: ModulesConfig{
			Vision: ModuleEntryConfig{Enabled: &f, Provider: "local"},
		},
		Vision: VisionConfig{Enabled: true},
	}
	cfg.applyDefaults()
	if cfg.IsVisionModuleEnabled() {
		t.Fatal("vision should be disabled via modules.vision.enabled=false")
	}
	if cfg.Vision.Enabled {
		t.Fatal("legacy vision.enabled should sync to false")
	}
}

func TestResolvedModules_defaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	m := cfg.ResolvedModules()
	if !m.ASR.Enabled || !m.TTS.Enabled || !m.LLM.Enabled {
		t.Fatal("default modules should be enabled")
	}
	if m.LLM.Provider != "remote" {
		t.Fatalf("llm provider=%s want remote", m.LLM.Provider)
	}
}
