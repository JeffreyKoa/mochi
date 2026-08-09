// Package sidecarlog 记录 Go 主服务对本地 sidecar 的接口调用（写入 logs/mochi/*.log）。
package sidecarlog

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	maxBodyLog          = 512
	wsAudioSampleEvery  = 50   // 流式 PCM 每 N 个 chunk 采样一条
	wsAudioBatchChunk   = 640  // 与 Desktop/x-asr 一致的 20ms@16kHz mono int16
	wsAudioLargeBytes   = 8192 // 单包超过此大小视为 bulk，单独记一条
)

// WSSessionStats 单次 sidecar WS 会话的汇总（用于排查 ASR/TTS 链路）。
type WSSessionStats struct {
	AudioChunks    int
	AudioBytes     int
	EmptyPartials  int
	TextPartials   int
	LastPartial    string
	FinalText      string
	StartedWaitMS  int64
	ElapsedMS      int64
}

type wsStreamKey struct {
	service, url string
}

type wsAudioAgg struct {
	chunks     int
	bytes      int
	firstLogAt time.Time
}

var (
	wsAggMu     sync.Mutex
	wsAudioAggs = map[wsStreamKey]*wsAudioAgg{}
)

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

// LogWSOutbound 记录 WebSocket 出站 JSON 控制帧。
func LogWSOutbound(service, url, kind string, payload any) {
	log.Printf("[sidecar][%s] WS -> %s %s payload=%s", service, url, kind, formatPayload(payload))
}

// LogWSOutboundPCM 记录出站 PCM：小 chunk 采样聚合，bulk 单包单独记。
func LogWSOutboundPCM(service, url string, pcmLen int, mode string) {
	if pcmLen <= 0 {
		return
	}
	key := wsStreamKey{service, url}
	wsAggMu.Lock()
	agg := wsAudioAggs[key]
	if agg == nil {
		agg = &wsAudioAgg{firstLogAt: time.Now()}
		wsAudioAggs[key] = agg
	}
	agg.chunks++
	agg.bytes += pcmLen
	chunks := agg.chunks
	bytes := agg.bytes
	wsAggMu.Unlock()

	// bulk 或首包必记；其余按采样间隔记
	if pcmLen >= wsAudioLargeBytes || chunks == 1 || chunks%wsAudioSampleEvery == 0 {
		log.Printf("[sidecar][%s] WS -> %s audio mode=%s chunk=%d chunk_bytes=%d total_chunks=%d total_bytes=%d",
			service, url, mode, chunks, pcmLen, chunks, bytes)
	}
}

// LogWSBatchPCMStart 批量识别开始分块发送前记一条汇总。
func LogWSBatchPCMStart(service, url string, totalBytes, chunkBytes int) {
	log.Printf("[sidecar][%s] WS -> %s audio mode=batch_start total_bytes=%d chunk_bytes=%d est_chunks=%d",
		service, url, totalBytes, chunkBytes, (totalBytes+chunkBytes-1)/chunkBytes)
}

// LogWSInbound 记录 WebSocket 入站消息（非空 partial / final / started / error）。
func LogWSInbound(service, url, kind string, payload any) {
	log.Printf("[sidecar][%s] WS <- %s %s payload=%s", service, url, kind, formatPayload(payload))
}

// LogWSSessionSummary 会话结束汇总：便于对照「发了多少音频、收到多少 partial、final 是否为空」。
func LogWSSessionSummary(service, url, reason string, st WSSessionStats) {
	wsAggMu.Lock()
	delete(wsAudioAggs, wsStreamKey{service, url})
	wsAggMu.Unlock()
	log.Printf("[sidecar][%s] WS session summary %s reason=%s audio_chunks=%d audio_bytes=%d empty_partials=%d text_partials=%d last_partial=%q final=%q started_wait_ms=%d elapsed_ms=%d",
		service, url, reason,
		st.AudioChunks, st.AudioBytes, st.EmptyPartials, st.TextPartials,
		truncate(st.LastPartial, 80), truncate(st.FinalText, 120),
		st.StartedWaitMS, st.ElapsedMS)
}

// WSBatchChunkBytes 返回批量发送推荐 chunk 大小。
func WSBatchChunkBytes() int { return wsAudioBatchChunk }

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
