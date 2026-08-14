package capability

import (
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func boolPtr(b bool) *bool { return &b }

func testConfig(modules config.ModulesConfig, extra ...func(*config.Config)) *config.Config {
	cfg := &config.Config{Modules: modules}
	for _, fn := range extra {
		fn(cfg)
	}
	cfg.Capability = config.CapabilityConfig{
		ASR:     config.ModuleThresholds{MinRAMMB: 4096},
		TTS:     config.ModuleThresholds{MinRAMMB: 4096},
		Vision:  config.ModuleThresholds{MinRAMMB: 8192, MinVRAMMB: 4096},
		Emotion: config.ModuleThresholds{MinRAMMB: 4096},
	}
	return cfg
}

func TestEvaluate_disabledModules(t *testing.T) {
	cfg := testConfig(config.ModulesConfig{
		ASR:     config.ModuleEntryConfig{Enabled: boolPtr(false), Provider: "local"},
		TTS:     config.ModuleEntryConfig{Enabled: boolPtr(false), Provider: "local"},
		LLM:     config.ModuleEntryConfig{Enabled: boolPtr(false), Provider: "remote"},
		Vision:  config.ModuleEntryConfig{Enabled: boolPtr(false), Provider: "local"},
		Emotion: config.ModuleEntryConfig{Enabled: boolPtr(false), Provider: "local"},
	})

	report := Evaluate(HardwareSnapshot{RAMTotalMB: 16384, GPU: GPUInfo{Present: true, VRAMTotalMB: 8192}}, cfg)
	for _, m := range report.Modules {
		if m.Status != StatusDisabled {
			t.Fatalf("module %s expected disabled, got %s", m.Name, m.Status)
		}
	}
}

func TestEvaluate_visionNoGPU_degraded(t *testing.T) {
	cfg := testConfig(
		config.ModulesConfig{
			Vision: config.ModuleEntryConfig{Enabled: boolPtr(true), Provider: "local"},
		},
		func(c *config.Config) {
			c.Vision = config.VisionConfig{Enabled: true, Backend: "local_moondream"}
		},
	)

	report := Evaluate(HardwareSnapshot{
		RAMTotalMB: 16384,
		GPU:        GPUInfo{Present: false},
	}, cfg)

	var vision ModuleReport
	for _, m := range report.Modules {
		if m.Name == ModuleVision {
			vision = m
		}
	}
	if vision.Status != StatusDegraded {
		t.Fatalf("vision status=%s want degraded", vision.Status)
	}
}

func TestEvaluate_visionLowRAM_unsupported(t *testing.T) {
	cfg := testConfig(config.ModulesConfig{
		Vision: config.ModuleEntryConfig{Enabled: boolPtr(true), Provider: "local"},
	})

	report := Evaluate(HardwareSnapshot{RAMTotalMB: 2048}, cfg)
	for _, m := range report.Modules {
		if m.Name == ModuleVision && m.Status != StatusUnsupported {
			t.Fatalf("vision status=%s want unsupported", m.Status)
		}
	}
}

func TestEvaluate_llmMissingKey(t *testing.T) {
	cfg := testConfig(config.ModulesConfig{
		LLM: config.ModuleEntryConfig{Enabled: boolPtr(true), Provider: "remote"},
	})

	report := Evaluate(HardwareSnapshot{}, cfg)
	for _, m := range report.Modules {
		if m.Name == ModuleLLM && m.Status != StatusRemoteMissing {
			t.Fatalf("llm status=%s want remote_missing", m.Status)
		}
	}
}

func TestEvaluate_asrOK(t *testing.T) {
	cfg := testConfig(config.ModulesConfig{
		ASR: config.ModuleEntryConfig{Enabled: boolPtr(true), Provider: "local"},
	})

	report := Evaluate(HardwareSnapshot{RAMTotalMB: 8192}, cfg)
	for _, m := range report.Modules {
		if m.Name == ModuleASR && m.Status != StatusOK {
			t.Fatalf("asr status=%s want ok", m.Status)
		}
	}
}
