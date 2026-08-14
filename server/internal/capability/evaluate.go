package capability

import (
	"strings"

	"github.com/mochi-ai/server/internal/config"
)

// Evaluate 根据硬件快照与配置，评估各模块能力状态。
func Evaluate(hw HardwareSnapshot, cfg *config.Config) Report {
	if cfg == nil {
		return Report{Hardware: hw}
	}
	resolved := cfg.ResolvedModules()
	thresholds := cfg.Capability

	reports := []ModuleReport{
		evaluateASR(hw, resolved.ASR, thresholds.ASR),
		evaluateTTS(hw, resolved.TTS, thresholds.TTS),
		evaluateLLM(resolved.LLM, cfg.AI.APIKey != ""),
		evaluateVision(hw, resolved.Vision, thresholds.Vision),
		evaluateEmotion(hw, resolved.Emotion, thresholds.Emotion),
	}
	return Report{Hardware: hw, Modules: reports}
}

func evaluateASR(hw HardwareSnapshot, m config.ResolvedModule, th config.ModuleThresholds) ModuleReport {
	r := baseReport(ModuleASR, m, th, needsLocalSidecar(m.Provider, "xasr"))
	if !m.Enabled {
		r.Status = StatusDisabled
		r.Reason = "模块已关闭"
		return r
	}
	if isRemoteOnly(m.Provider) {
		r.Status = StatusRemoteReady
		r.Reason = "使用远程 ASR API"
		return r
	}
	return evaluateLocalRAM(hw, r, th)
}

func evaluateTTS(hw HardwareSnapshot, m config.ResolvedModule, th config.ModuleThresholds) ModuleReport {
	r := baseReport(ModuleTTS, m, th, needsLocalSidecar(m.Provider, "xtts"))
	if !m.Enabled {
		r.Status = StatusDisabled
		r.Reason = "模块已关闭"
		return r
	}
	if isRemoteOnly(m.Provider) {
		r.Status = StatusRemoteReady
		r.Reason = "使用远程 TTS API"
		return r
	}
	return evaluateLocalRAM(hw, r, th)
}

func evaluateLLM(m config.ResolvedModule, hasAPIKey bool) ModuleReport {
	r := ModuleReport{
		Name:                 ModuleLLM,
		Enabled:              m.Enabled,
		Provider:             m.Provider,
		RequiresLocalSidecar: false,
	}
	if !m.Enabled {
		r.Status = StatusDisabled
		r.Reason = "模块已关闭"
		return r
	}
	switch strings.ToLower(m.Provider) {
	case "local":
		r.Status = StatusDegraded
		r.Reason = "本地 LLM 尚未接入（Phase 4b）"
	case "remote":
		if hasAPIKey {
			r.Status = StatusRemoteReady
			r.Reason = "远程 API 已配置"
		} else {
			r.Status = StatusRemoteMissing
			r.Reason = "请配置 ai.api_key"
		}
	default: // auto
		if hasAPIKey {
			r.Status = StatusRemoteReady
			r.Reason = "auto：当前使用远程 API"
		} else {
			r.Status = StatusRemoteMissing
			r.Reason = "auto：无 API Key，请配置 ai.api_key 或接入本地 LLM"
		}
	}
	return r
}

func evaluateVision(hw HardwareSnapshot, m config.ResolvedModule, th config.ModuleThresholds) ModuleReport {
	r := baseReport(ModuleVision, m, th, needsLocalSidecar(m.Provider, "local_moondream"))
	if !m.Enabled {
		r.Status = StatusDisabled
		r.Reason = "模块已关闭"
		return r
	}
	if isRemoteOnly(m.Provider) {
		r.Status = StatusRemoteReady
		r.Reason = "使用远程视觉 API（Qwen-VL 等）"
		return r
	}
	// 本地 moondream：无 GPU 时 CPU 可跑但降级
	if hw.RAMTotalMB > 0 && hw.RAMTotalMB < int64(th.MinRAMMB) {
		if hw.RAMTotalMB < int64(th.MinRAMMB/2) {
			r.Status = StatusUnsupported
			r.Reason = "内存不足，无法运行视觉模型"
			return r
		}
	}
	if hw.GPU.Present && hw.GPU.VRAMTotalMB >= int64(th.MinVRAMMB) {
		r.Status = StatusOK
		r.Reason = "GPU 可用，推荐本地视觉"
		return r
	}
	if hw.RAMTotalMB >= int64(th.MinRAMMB) {
		r.Status = StatusDegraded
		r.Reason = "无可用 GPU 或显存不足，将使用 CPU 运行（较慢）"
		return r
	}
	if hw.RAMTotalMB == 0 {
		r.Status = StatusDegraded
		r.Reason = "未能探测内存，假定 CPU 模式"
		return r
	}
	r.Status = StatusUnsupported
	r.Reason = "内存不足，无法运行视觉模型"
	return r
}

func evaluateEmotion(hw HardwareSnapshot, m config.ResolvedModule, th config.ModuleThresholds) ModuleReport {
	r := baseReport(ModuleEmotion, m, th, true)
	if !m.Enabled {
		r.Status = StatusDisabled
		r.Reason = "模块已关闭"
		return r
	}
	if hw.RAMTotalMB > 0 && hw.RAMTotalMB < int64(th.MinRAMMB/2) {
		r.Status = StatusUnsupported
		r.Reason = "内存不足"
		return r
	}
	if hw.GPU.Present && hw.GPU.VRAMTotalMB > 0 && hw.GPU.VRAMTotalMB <= 6144 {
		r.Status = StatusDegraded
		r.Reason = "显存≤6GB，emotion2vec 将使用 CPU"
		return r
	}
	if hw.RAMTotalMB >= int64(th.MinRAMMB) || hw.RAMTotalMB == 0 {
		r.Status = StatusOK
		if hw.GPU.Present {
			r.Reason = "本地情感识别可用"
		} else {
			r.Reason = "本地情感识别可用（CPU）"
		}
		return r
	}
	r.Status = StatusDegraded
	r.Reason = "内存偏低，可能影响性能"
	return r
}

func baseReport(name ModuleName, m config.ResolvedModule, th config.ModuleThresholds, localSidecar bool) ModuleReport {
	_ = th
	return ModuleReport{
		Name:                 name,
		Enabled:              m.Enabled,
		Provider:             m.Provider,
		RequiresLocalSidecar: localSidecar,
	}
}

func evaluateLocalRAM(hw HardwareSnapshot, r ModuleReport, th config.ModuleThresholds) ModuleReport {
	minRAM := int64(th.MinRAMMB)
	if minRAM <= 0 {
		minRAM = 4096
	}
	if hw.RAMTotalMB == 0 {
		r.Status = StatusOK
		r.Reason = "未能探测内存，假定可运行"
		return r
	}
	if hw.RAMTotalMB < minRAM/2 {
		r.Status = StatusUnsupported
		r.Reason = "内存不足"
		return r
	}
	if hw.RAMTotalMB < minRAM {
		r.Status = StatusDegraded
		r.Reason = "内存偏低，可能影响性能"
		return r
	}
	r.Status = StatusOK
	r.Reason = "本地 sidecar 可运行"
	return r
}

func needsLocalSidecar(provider, localName string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "remote", "dashscope", "cloud_api":
		return false
	case "local", localName, "xasr", "xtts", "local_moondream", "moondream":
		return true
	default: // auto
		return true
	}
}

func isRemoteOnly(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "remote", "dashscope", "dashscope_vl", "cloud_api":
		return true
	default:
		return false
	}
}
