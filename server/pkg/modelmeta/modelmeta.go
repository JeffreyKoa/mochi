// Package modelmeta 统一记录 AI 调用的厂商与模型名称，便于日志排查。
package modelmeta

import (
	"log"
	"strings"
)

// 常见厂商标识（日志 vendor 字段）。
const (
	VendorAliyunDashScope = "aliyun_dashscope"
	VendorVolcengine      = "volcengine"
	VendorOpenAI          = "openai"
	VendorLocalXASR        = "local_xasr"
	VendorLocalXTTS        = "local_xtts"
	VendorLocalEmotion2vec = "local_emotion2vec"
	VendorCompatibleAPI   = "compatible_api"
)

// InferVendorFromAPIBase 根据 OpenAI 兼容 API 基址推断厂商。
func InferVendorFromAPIBase(apiBase string) string {
	u := strings.ToLower(strings.TrimSpace(apiBase))
	switch {
	case strings.Contains(u, "dashscope.aliyuncs.com"),
		strings.Contains(u, "maas.aliyuncs.com"):
		return VendorAliyunDashScope
	case strings.Contains(u, "volces.com"):
		return VendorVolcengine
	case strings.Contains(u, "openai.com"):
		return VendorOpenAI
	default:
		return VendorCompatibleAPI
	}
}

// LogCall 写入统一格式的模型调用日志（同时输出到 logs/mochi-*.log 与控制台）。
func LogCall(subsystem, vendor, model string, extras ...string) {
	vendor = strings.TrimSpace(vendor)
	model = strings.TrimSpace(model)
	if vendor == "" {
		vendor = "unknown"
	}
	if model == "" {
		model = "unknown"
	}
	if len(extras) > 0 {
		log.Printf("[model] subsystem=%s vendor=%s model=%s %s", subsystem, vendor, model, strings.Join(extras, " "))
		return
	}
	log.Printf("[model] subsystem=%s vendor=%s model=%s", subsystem, vendor, model)
}
