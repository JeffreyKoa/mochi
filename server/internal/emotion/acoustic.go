package emotion

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mochi-ai/server/pkg/modelmeta"
	"github.com/mochi-ai/server/pkg/sidecarlog"
)

// AcousticHint 声学情绪识别结果（emotion2vec 旁路）。
type AcousticHint struct {
	Mood       string             `json:"mood"`
	Confidence float64            `json:"confidence"`
	Label      string             `json:"label,omitempty"`
	Scores     map[string]float64 `json:"scores,omitempty"`
}

// EmptyAcousticHint 表示未启用或识别失败时的空结果。
func EmptyAcousticHint() AcousticHint {
	return AcousticHint{Mood: "neutral", Confidence: 0}
}

// AcousticClient 声学情绪识别客户端。
type AcousticClient interface {
	Recognize(ctx context.Context, pcm []byte, sampleRate int) (AcousticHint, error)
	Enabled() bool
}

// NoopAcousticClient 禁用时的空实现。
type NoopAcousticClient struct{}

func (NoopAcousticClient) Recognize(_ context.Context, _ []byte, _ int) (AcousticHint, error) {
	return EmptyAcousticHint(), nil
}

func (NoopAcousticClient) Enabled() bool { return false }

// HTTPAcousticClient 调用 emotion2vec Sidecar HTTP API。
type HTTPAcousticClient struct {
	baseURL    string
	httpClient *http.Client
	sampleRate int
}

// NewHTTPAcousticClient 创建 HTTP 声学客户端。
func NewHTTPAcousticClient(baseURL string, timeout time.Duration, sampleRate int) *HTTPAcousticClient {
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	return &HTTPAcousticClient{
		baseURL: stringsTrimRightSlash(baseURL),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		sampleRate: sampleRate,
	}
}

func (c *HTTPAcousticClient) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *HTTPAcousticClient) Recognize(ctx context.Context, pcm []byte, sampleRate int) (AcousticHint, error) {
	if !c.Enabled() || len(pcm) == 0 {
		return EmptyAcousticHint(), nil
	}
	modelmeta.LogCall("emotion_acoustic", modelmeta.VendorLocalEmotion2vec, "emotion2vec")
	if sampleRate <= 0 {
		sampleRate = c.sampleRate
	}

	body, err := json.Marshal(map[string]any{
		"pcm_base64":  base64.StdEncoding.EncodeToString(pcm),
		"sample_rate": sampleRate,
	})
	if err != nil {
		return EmptyAcousticHint(), err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/emotion", bytes.NewReader(body))
	if err != nil {
		return EmptyAcousticHint(), err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		sidecarlog.LogHTTP("emotion2vec", http.MethodPost, c.baseURL+"/v1/emotion", body, 0, nil, err, elapsed)
		return EmptyAcousticHint(), err
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if readErr != nil {
		sidecarlog.LogHTTP("emotion2vec", http.MethodPost, c.baseURL+"/v1/emotion", body, resp.StatusCode, raw, readErr, elapsed)
		return EmptyAcousticHint(), readErr
	}

	if resp.StatusCode != http.StatusOK {
		sidecarlog.LogHTTP("emotion2vec", http.MethodPost, c.baseURL+"/v1/emotion", body, resp.StatusCode, raw, fmt.Errorf("acoustic http %d", resp.StatusCode), elapsed)
		return EmptyAcousticHint(), fmt.Errorf("acoustic http %d: %s", resp.StatusCode, string(raw))
	}

	var out AcousticHint
	if err := json.Unmarshal(raw, &out); err != nil {
		sidecarlog.LogHTTP("emotion2vec", http.MethodPost, c.baseURL+"/v1/emotion", body, resp.StatusCode, raw, err, elapsed)
		return EmptyAcousticHint(), err
	}
	if out.Mood == "" {
		out.Mood = "neutral"
	}
	sidecarlog.LogHTTP("emotion2vec", http.MethodPost, c.baseURL+"/v1/emotion", body, resp.StatusCode, raw, nil, elapsed)
	return out, nil
}

func stringsTrimRightSlash(s string) string {
	return strings.TrimRight(s, "/")
}
