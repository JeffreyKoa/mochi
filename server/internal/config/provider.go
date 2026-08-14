package config

import "strings"

// NormalizeModuleProvider 统一模块 provider 语义。
func NormalizeModuleProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "auto":
		return "auto"
	case "local", "xasr", "xtts", "sherpa", "matcha", "local_moondream", "moondream":
		return "local"
	case "remote", "dashscope", "dashscope_vl", "cloud", "cloud_api":
		return "remote"
	default:
		return strings.ToLower(strings.TrimSpace(p))
	}
}

// EffectiveASRProvider 合并 modules 与 legacy realtime.asr.provider。
func (c *Config) EffectiveASRProvider() string {
	if c == nil {
		return "auto"
	}
	m := c.ResolvedModules().ASR
	if !m.Enabled {
		return "none"
	}
	p := NormalizeModuleProvider(m.Provider)
	if p != "auto" {
		return p
	}
	legacy := strings.ToLower(strings.TrimSpace(c.Realtime.ASR.Provider))
	switch legacy {
	case "none":
		return "none"
	case "xasr", "sherpa", "local":
		return "local"
	case "dashscope", "remote":
		return "remote"
	default:
		return "auto"
	}
}

// EffectiveTTSProvider 合并 modules 与 legacy realtime.tts.provider。
func (c *Config) EffectiveTTSProvider() string {
	if c == nil {
		return "auto"
	}
	m := c.ResolvedModules().TTS
	if !m.Enabled {
		return "none"
	}
	p := NormalizeModuleProvider(m.Provider)
	if p != "auto" {
		return p
	}
	legacy := strings.ToLower(strings.TrimSpace(c.Realtime.TTS.Provider))
	switch legacy {
	case "none":
		return "none"
	case "xtts", "matcha", "local":
		return "local"
	case "dashscope", "remote":
		return "remote"
	default:
		return "auto"
	}
}

// EffectiveVisionProvider 合并 modules 与 legacy vision.backend。
func (c *Config) EffectiveVisionProvider() string {
	if c == nil {
		return "auto"
	}
	m := c.ResolvedModules().Vision
	if !m.Enabled {
		return "none"
	}
	p := NormalizeModuleProvider(m.Provider)
	if p != "auto" {
		return p
	}
	switch strings.ToLower(strings.TrimSpace(c.Vision.Backend)) {
	case "dashscope_vl", "remote":
		return "remote"
	case "local_moondream", "moondream", "local":
		return "local"
	default:
		return "auto"
	}
}

// HasRemoteAPIKey 是否配置了远程 API Key。
func (c *Config) HasRemoteAPIKey() bool {
	return c != nil && strings.TrimSpace(c.AI.APIKey) != ""
}
