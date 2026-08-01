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
)

type vlClient struct {
	apiBase string
	apiKey  string
	model   string
	client  *http.Client
}

func newVLClient(apiBase, apiKey, model string, timeout time.Duration) *vlClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &vlClient{
		apiBase: strings.TrimRight(apiBase, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

type vlMessage struct {
	Role    string      `json:"role"`
	Content []vlContent `json:"content"`
}

type vlContent struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *vlImageURL `json:"image_url,omitempty"`
}

type vlImageURL struct {
	URL string `json:"url"`
}

type vlRequest struct {
	Model    string      `json:"model"`
	Messages []vlMessage `json:"messages"`
}

type vlResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *vlClient) chat(ctx context.Context, jpeg []byte, userPrompt string) (string, error) {
	if len(jpeg) == 0 {
		return "", fmt.Errorf("empty jpeg")
	}
	b64 := base64.StdEncoding.EncodeToString(jpeg)
	dataURL := "data:image/jpeg;base64," + b64

	reqBody := vlRequest{
		Model: c.model,
		Messages: []vlMessage{{
			Role: "user",
			Content: []vlContent{
				{Type: "image_url", ImageURL: &vlImageURL{URL: dataURL}},
				{Type: "text", Text: userPrompt},
			},
		}},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := c.apiBase + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vl http %d: %s", resp.StatusCode, truncateBytes(raw, 512))
	}

	var out vlResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("vl decode: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("vl api: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("vl empty choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
