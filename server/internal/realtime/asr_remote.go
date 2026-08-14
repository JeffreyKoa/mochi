package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/modelmeta"
)

// remoteASR 通过 OpenAI 兼容 POST /audio/transcriptions 做批量识别。
type remoteASR struct {
	apiBase    string
	apiKey     string
	model      string
	sampleRate int
	client     *http.Client
}

func newRemoteASR(ai config.AIConfig, sampleRate int) ASRRecognizer {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	timeout := 60 * time.Second
	return &remoteASR{
		apiBase:    strings.TrimRight(strings.TrimSpace(ai.APIBase), "/"),
		apiKey:     strings.TrimSpace(ai.APIKey),
		model:      strings.TrimSpace(ai.ASRModel),
		sampleRate: sampleRate,
		client:     &http.Client{Timeout: timeout},
	}
}

func (r *remoteASR) Recognize(ctx context.Context, pcm []byte, onPartial ASRPartialHandler) (string, error) {
	_ = onPartial
	if len(pcm) == 0 {
		return "", fmt.Errorf("empty pcm")
	}
	if r.apiKey == "" {
		return "", fmt.Errorf("remote ASR: ai.api_key not configured")
	}
	wav := pcm16ToWAV(pcm, r.sampleRate)
	modelmeta.LogCall("asr", modelmeta.InferVendorFromAPIBase(r.apiBase), r.model)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("model", r.model)
	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	_ = w.Close()

	url := r.apiBase + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote asr: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("remote asr http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	text := parseTranscriptionResponse(raw)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("remote asr empty transcript")
	}
	return text, nil
}

func (r *remoteASR) StartSession(ctx context.Context, onPartial ASRPartialHandler) (ASRSession, error) {
	// 远程 ASR 无流式 session；由 chainASR 在失败时走 batch Recognize。
	return nil, fmt.Errorf("remote ASR does not support streaming session")
}

func parseTranscriptionResponse(raw []byte) string {
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Text != "" {
		return obj.Text
	}
	return strings.TrimSpace(string(raw))
}
