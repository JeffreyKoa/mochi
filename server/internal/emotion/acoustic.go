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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return EmptyAcousticHint(), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return EmptyAcousticHint(), fmt.Errorf("acoustic http %d: %s", resp.StatusCode, string(b))
	}

	var out AcousticHint
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return EmptyAcousticHint(), err
	}
	if out.Mood == "" {
		out.Mood = "neutral"
	}
	return out, nil
}

func stringsTrimRightSlash(s string) string {
	return strings.TrimRight(s, "/")
}
