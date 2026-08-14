package capability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/config"
)

const setupProbeTimeout = 2 * time.Second

// ProbeSetup 探测各 enabled 模块的 sidecar 健康状态。
func ProbeSetup(ctx context.Context, cfg *config.Config) SetupReport {
	if cfg == nil {
		return SetupReport{}
	}
	resolved := cfg.ResolvedModules()
	items := []SidecarStatus{
		probeSidecar(ctx, ModuleASR, resolved.ASR, asrHealthURL(cfg), shouldProbeLocalASR(resolved.ASR)),
		probeSidecar(ctx, ModuleTTS, resolved.TTS, strings.TrimRight(cfg.Realtime.XTTS.BaseURL, "/")+"/health", shouldProbeLocalTTS(resolved.TTS)),
		probeSidecar(ctx, ModuleVision, resolved.Vision, strings.TrimRight(cfg.Vision.SidecarURL, "/")+"/health", shouldProbeLocalVision(resolved.Vision, cfg.Vision.Backend)),
		probeSidecar(ctx, ModuleEmotion, resolved.Emotion, strings.TrimRight(cfg.Emotion.Acoustic.URL, "/")+"/health", resolved.Emotion.Enabled),
	}
	return SetupReport{Modules: items}
}

func asrHealthURL(cfg *config.Config) string {
	ws := strings.TrimSpace(cfg.Realtime.XASR.WSURL)
	if ws == "" {
		ws = "ws://127.0.0.1:8766"
	}
	if strings.HasPrefix(strings.ToLower(ws), "ws://") {
		host := strings.TrimPrefix(strings.ToLower(ws), "ws://")
		return "http://" + strings.TrimRight(host, "/") + "/health"
	}
	return strings.TrimRight(ws, "/") + "/health"
}

func shouldProbeLocalASR(m config.ResolvedModule) bool {
	if !m.Enabled {
		return false
	}
	return config.NeedsLocalSidecar(m.Provider)
}

func shouldProbeLocalTTS(m config.ResolvedModule) bool {
	if !m.Enabled {
		return false
	}
	return config.NeedsLocalSidecar(m.Provider)
}

func shouldProbeLocalVision(m config.ResolvedModule, backend string) bool {
	if !m.Enabled {
		return false
	}
	if isRemoteOnly(m.Provider) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "dashscope_vl", "remote":
		return false
	default:
		return true
	}
}

func probeSidecar(ctx context.Context, name ModuleName, m config.ResolvedModule, healthURL string, probeLocal bool) SidecarStatus {
	st := SidecarStatus{
		Name:    name,
		Enabled: m.Enabled,
		URL:     healthURL,
	}
	if !m.Enabled {
		st.Status = StatusDisabled
		st.Reason = "模块已关闭"
		return st
	}
	if !probeLocal {
		st.Status = StatusRemoteReady
		st.Reason = "远程模式，无需本地 sidecar"
		st.Healthy = true
		st.ModelReady = true
		return st
	}
	if strings.TrimSpace(healthURL) == "" {
		st.Status = StatusUnsupported
		st.Reason = "未配置 sidecar 地址"
		return st
	}

	reqCtx, cancel := context.WithTimeout(ctx, setupProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		st.Status = StatusUnsupported
		st.Reason = err.Error()
		return st
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		st.Status = StatusDegraded
		st.Reason = "sidecar 未响应: " + err.Error()
		return st
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		st.Healthy = true
		st.ModelReady = parseModelReady(body)
		st.Status = StatusOK
		if !st.ModelReady {
			st.Status = StatusDegraded
			st.Reason = "sidecar 已启动，模型可能尚未加载"
		}
		return st
	}
	st.Status = StatusDegraded
	st.Reason = "health 非 2xx"
	return st
}

func parseModelReady(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return true
	}
	for _, key := range []string{"model_ready", "ready", "loaded"} {
		if v, ok := m[key]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
	}
	if status, ok := m["status"].(string); ok {
		return strings.EqualFold(status, "ok") || strings.EqualFold(status, "ready")
	}
	return true
}
