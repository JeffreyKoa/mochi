package vision

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
)

// localMoondreamClient 调用本地 Moondream sidecar（JPEG + prompt → 文本，Go 侧 parseVLResponse）。
type localMoondreamClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func newLocalMoondreamClient(baseURL, model string, timeout time.Duration) *localMoondreamClient {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if model == "" {
		model = "moondream2"
	}
	return &localMoondreamClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

type moondreamDescribeReq struct {
	JPEGBase64 string `json:"jpeg_base64"`
	Prompt     string `json:"prompt"`
}

type moondreamDescribeResp struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

func (c *localMoondreamClient) chat(ctx context.Context, jpeg []byte, userPrompt string) (string, error) {
	if len(jpeg) == 0 {
		return "", fmt.Errorf("empty jpeg")
	}
	if strings.TrimSpace(userPrompt) == "" {
		return "", fmt.Errorf("empty prompt")
	}
	modelmeta.LogCall("vision_moondream", "local_moondream", c.model)

	body, err := json.Marshal(moondreamDescribeReq{
		JPEGBase64: base64.StdEncoding.EncodeToString(jpeg),
		Prompt:     userPrompt,
	})
	if err != nil {
		return "", err
	}

	url := c.baseURL + "/v1/describe"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("moondream http %d: %s", resp.StatusCode, truncateBytes(raw, 512))
	}

	var out moondreamDescribeResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("moondream decode: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("moondream: %s", out.Error)
	}
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return "", fmt.Errorf("moondream empty text")
	}
	return text, nil
}
