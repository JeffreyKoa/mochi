package config

import "strings"

// ModuleEntryConfig 单模块开关与路由（modules.*）。
type ModuleEntryConfig struct {
	Enabled  *bool  `yaml:"enabled"`
	Provider string `yaml:"provider"` // local | remote | auto
}

// ModulesConfig 各 AI 模块统一开关。
type ModulesConfig struct {
	ASR     ModuleEntryConfig `yaml:"asr"`
	TTS     ModuleEntryConfig `yaml:"tts"`
	LLM     ModuleEntryConfig `yaml:"llm"`
	Vision  ModuleEntryConfig `yaml:"vision"`
	Emotion ModuleEntryConfig `yaml:"emotion"`
}

// ModuleThresholds 单模块硬件门槛（capability.*）。
type ModuleThresholds struct {
	MinRAMMB  int `yaml:"min_ram_mb"`
	MinVRAMMB int `yaml:"min_vram_mb"`
}

// CapabilityConfig 硬件能力评估阈值。
type CapabilityConfig struct {
	ASR     ModuleThresholds `yaml:"asr"`
	TTS     ModuleThresholds `yaml:"tts"`
	Vision  ModuleThresholds `yaml:"vision"`
	Emotion ModuleThresholds `yaml:"emotion"`
}

// ResolvedModule 合并 legacy 字段后的有效模块配置。
type ResolvedModule struct {
	Enabled  bool
	Provider string
}

// ResolvedModules 全部模块的有效配置。
type ResolvedModules struct {
	ASR     ResolvedModule
	TTS     ResolvedModule
	LLM     ResolvedModule
	Vision  ResolvedModule
	Emotion ResolvedModule
}

func (m ModuleEntryConfig) enabledOr(defaultVal bool) bool {
	if m.Enabled == nil {
		return defaultVal
	}
	return *m.Enabled
}

func (c *CapabilityConfig) applyDefaults() {
	if c.ASR.MinRAMMB == 0 {
		c.ASR.MinRAMMB = 4096
	}
	if c.TTS.MinRAMMB == 0 {
		c.TTS.MinRAMMB = 4096
	}
	if c.Vision.MinRAMMB == 0 {
		c.Vision.MinRAMMB = 8192
	}
	if c.Vision.MinVRAMMB == 0 {
		c.Vision.MinVRAMMB = 4096
	}
	if c.Emotion.MinRAMMB == 0 {
		c.Emotion.MinRAMMB = 4096
	}
}

func (c *Config) syncModulesFromLegacy() {
	// ASR：legacy provider=none 视为关闭
	asrEnabled := c.Modules.ASR.enabledOr(true)
	if prov := strings.ToLower(strings.TrimSpace(c.Realtime.ASR.Provider)); prov == "none" {
		asrEnabled = false
	}
	if c.Modules.ASR.Enabled != nil {
		asrEnabled = *c.Modules.ASR.Enabled
	}
	asrProvider := c.Modules.ASR.Provider
	if asrProvider == "" {
		asrProvider = mapLegacyASRProvider(c.Realtime.ASR.Provider, asrEnabled)
	}
	if !asrEnabled {
		c.Realtime.ASR.Provider = "none"
	} else if strings.ToLower(c.Realtime.ASR.Provider) == "none" {
		c.Realtime.ASR.Provider = "xasr"
	}

	// TTS
	ttsEnabled := c.Modules.TTS.enabledOr(true)
	if prov := strings.ToLower(strings.TrimSpace(c.Realtime.TTS.Provider)); prov == "none" {
		ttsEnabled = false
	}
	if c.Modules.TTS.Enabled != nil {
		ttsEnabled = *c.Modules.TTS.Enabled
	}
	ttsProvider := c.Modules.TTS.Provider
	if ttsProvider == "" {
		ttsProvider = mapLegacyTTSProvider(c.Realtime.TTS.Provider, ttsEnabled)
	}
	if !ttsEnabled {
		c.Realtime.TTS.Provider = "none"
	} else if strings.ToLower(c.Realtime.TTS.Provider) == "none" {
		c.Realtime.TTS.Provider = "xtts"
	}

	// LLM
	llmEnabled := c.Modules.LLM.enabledOr(true)
	if c.Modules.LLM.Enabled != nil {
		llmEnabled = *c.Modules.LLM.Enabled
	}
	llmProvider := c.Modules.LLM.Provider
	if llmProvider == "" {
		llmProvider = "remote"
	}

	// Vision
	visionEnabled := c.Modules.Vision.enabledOr(c.Vision.Enabled)
	if c.Modules.Vision.Enabled != nil {
		visionEnabled = *c.Modules.Vision.Enabled
	}
	c.Vision.Enabled = visionEnabled
	visionProvider := c.Modules.Vision.Provider
	if visionProvider == "" {
		visionProvider = mapLegacyVisionProvider(c.Vision.Backend)
	}

	// Emotion
	emotionEnabled := c.Modules.Emotion.enabledOr(c.Emotion.Acoustic.Enabled)
	if c.Modules.Emotion.Enabled != nil {
		emotionEnabled = *c.Modules.Emotion.Enabled
	}
	c.Emotion.Acoustic.Enabled = emotionEnabled
	emotionProvider := c.Modules.Emotion.Provider
	if emotionProvider == "" {
		emotionProvider = "local"
	}

	// 写回规范化 modules（便于 API 返回）
	t := func(b bool) *bool { return &b }
	c.Modules.ASR.Enabled = t(asrEnabled)
	c.Modules.ASR.Provider = asrProvider
	c.Modules.TTS.Enabled = t(ttsEnabled)
	c.Modules.TTS.Provider = ttsProvider
	c.Modules.LLM.Enabled = t(llmEnabled)
	c.Modules.LLM.Provider = llmProvider
	c.Modules.Vision.Enabled = t(visionEnabled)
	c.Modules.Vision.Provider = visionProvider
	c.Modules.Emotion.Enabled = t(emotionEnabled)
	c.Modules.Emotion.Provider = emotionProvider
}

func mapLegacyASRProvider(legacy string, enabled bool) string {
	if !enabled {
		return "local"
	}
	switch strings.ToLower(strings.TrimSpace(legacy)) {
	case "none":
		return "local"
	case "xasr", "sherpa", "local":
		return "local"
	default:
		return "auto"
	}
}

func mapLegacyTTSProvider(legacy string, enabled bool) string {
	if !enabled {
		return "local"
	}
	switch strings.ToLower(strings.TrimSpace(legacy)) {
	case "none":
		return "local"
	case "xtts", "local":
		return "local"
	default:
		return "auto"
	}
}

func mapLegacyVisionProvider(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "dashscope_vl", "remote":
		return "remote"
	case "local_moondream", "moondream", "local":
		return "local"
	default:
		return "auto"
	}
}

// ResolvedModules 返回合并 legacy 后的有效模块配置。
func (c *Config) ResolvedModules() ResolvedModules {
	if c == nil {
		return ResolvedModules{}
	}
	return ResolvedModules{
		ASR: ResolvedModule{
			Enabled:  c.Modules.ASR.enabledOr(true),
			Provider: defaultStr(c.Modules.ASR.Provider, "auto"),
		},
		TTS: ResolvedModule{
			Enabled:  c.Modules.TTS.enabledOr(true),
			Provider: defaultStr(c.Modules.TTS.Provider, "auto"),
		},
		LLM: ResolvedModule{
			Enabled:  c.Modules.LLM.enabledOr(true),
			Provider: defaultStr(c.Modules.LLM.Provider, "remote"),
		},
		Vision: ResolvedModule{
			Enabled:  c.Modules.Vision.enabledOr(c.Vision.Enabled),
			Provider: defaultStr(c.Modules.Vision.Provider, "auto"),
		},
		Emotion: ResolvedModule{
			Enabled:  c.Modules.Emotion.enabledOr(c.Emotion.Acoustic.Enabled),
			Provider: defaultStr(c.Modules.Emotion.Provider, "local"),
		},
	}
}

// IsASREnabled ASR 模块是否启用。
func (c *Config) IsASREnabled() bool {
	return c.ResolvedModules().ASR.Enabled
}

// IsTTSEnabled TTS 模块是否启用。
func (c *Config) IsTTSEnabled() bool {
	return c.ResolvedModules().TTS.Enabled
}

// IsLLMEnabled LLM 模块是否启用。
func (c *Config) IsLLMEnabled() bool {
	return c.ResolvedModules().LLM.Enabled
}

// IsVisionModuleEnabled 视觉模块是否启用。
func (c *Config) IsVisionModuleEnabled() bool {
	return c.ResolvedModules().Vision.Enabled
}

// IsEmotionModuleEnabled 情感模块是否启用。
func (c *Config) IsEmotionModuleEnabled() bool {
	return c.ResolvedModules().Emotion.Enabled
}

// NeedsLocalSidecar 该模块在当前 provider 下是否需要本地 sidecar。
func NeedsLocalSidecar(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "remote", "dashscope", "dashscope_vl", "cloud_api":
		return false
	case "local", "xasr", "xtts", "local_moondream", "moondream":
		return true
	default:
		return true // auto 默认尝试本地
	}
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ModulePublicEntry 暴露给客户端的单模块开关（无密钥）。
type ModulePublicEntry struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
}

// ModulesPublicConfig 各 AI 模块开关摘要。
type ModulesPublicConfig struct {
	ASR     ModulePublicEntry `json:"asr"`
	TTS     ModulePublicEntry `json:"tts"`
	LLM     ModulePublicEntry `json:"llm"`
	Vision  ModulePublicEntry `json:"vision"`
	Emotion ModulePublicEntry `json:"emotion"`
}

// PublicModules 返回模块开关（供 GET /api/v1/public/config）。
func (c *Config) PublicModules() ModulesPublicConfig {
	r := c.ResolvedModules()
	return ModulesPublicConfig{
		ASR:     ModulePublicEntry{Enabled: r.ASR.Enabled, Provider: r.ASR.Provider},
		TTS:     ModulePublicEntry{Enabled: r.TTS.Enabled, Provider: r.TTS.Provider},
		LLM:     ModulePublicEntry{Enabled: r.LLM.Enabled, Provider: r.LLM.Provider},
		Vision:  ModulePublicEntry{Enabled: r.Vision.Enabled, Provider: r.Vision.Provider},
		Emotion: ModulePublicEntry{Enabled: r.Emotion.Enabled, Provider: r.Emotion.Provider},
	}
}
