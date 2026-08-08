// Package sidecarlog 记录 Go 主服务对本地 sidecar 的接口调用（写入 logs/mochi/*.log）。
package sidecarlog

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const maxBodyLog = 512

// LogHTTP 记录 HTTP sidecar 请求与响应摘要（大 payload 自动截断/脱敏）。
func LogHTTP(service, method, url string, reqBody []byte, status int, respBody []byte, err error, elapsed time.Duration) {
	reqSummary := summarizeReqBody(reqBody)
	respSummary := summarizeHTTPResp(status, respBody)
	ms := elapsed.Milliseconds()
	if err != nil {
		log.Printf("[sidecar][%s] %s %s elapsed_ms=%d req=%s resp=%s err=%v",
			service, method, url, ms, reqSummary, respSummary, err)
		return
	}
	log.Printf("[sidecar][%s] %s %s elapsed_ms=%d req=%s resp=%s",
		service, method, url, ms, reqSummary, respSummary)
}

// LogWSConnect 记录 WebSocket 连接建立。
func LogWSConnect(service, url string, err error) {
	if err != nil {
		log.Printf("[sidecar][%s] WS CONNECT %s err=%v", service, url, err)
		return
	}
	log.Printf("[sidecar][%s] WS CONNECT %s ok", service, url)
}

// LogWSOutbound 记录 WebSocket 出站 JSON 或二进制摘要。
func LogWSOutbound(service, url, kind string, payload any) {
	log.Printf("[sidecar][%s] WS -> %s %s payload=%s", service, url, kind, formatPayload(payload))
}

// LogWSInbound 记录 WebSocket 入站消息。
func LogWSInbound(service, url, kind string, payload any) {
	log.Printf("[sidecar][%s] WS <- %s %s payload=%s", service, url, kind, formatPayload(payload))
}

func summarizeReqBody(body []byte) string {
	if len(body) == 0 {
		return "{}"
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return truncate(string(body), maxBodyLog)
	}
	if b64, ok := m["pcm_base64"].(string); ok {
		m["pcm_base64"] = fmt.Sprintf("<base64 len=%d>", len(b64))
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return truncate(string(body), maxBodyLog)
	}
	return truncate(string(raw), maxBodyLog)
}

func summarizeHTTPResp(status int, body []byte) string {
	if status == 0 && len(body) == 0 {
		return "{}"
	}
	if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
		return fmt.Sprintf("status=%d body=%s", status, truncate(string(body), maxBodyLog))
	}
	if status == 0 {
		return fmt.Sprintf("body_bytes=%d", len(body))
	}
	return fmt.Sprintf("status=%d body_bytes=%d", status, len(body))
}

func formatPayload(payload any) string {
	switch v := payload.(type) {
	case string:
		return truncate(v, maxBodyLog)
	case []byte:
		return fmt.Sprintf("<bytes len=%d>", len(v))
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return truncate(string(raw), maxBodyLog)
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		max = maxBodyLog
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
