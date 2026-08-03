package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/agent"
	"github.com/mochi-ai/server/internal/config"
)

// ResponseGate decides whether an ASR transcript deserves an LLM response.
// It is fail-open: any error/timeout/parse failure lets the turn through.
type ResponseGate struct {
	apiKey        string
	apiBase       string
	model         string
	timeout       time.Duration
	maxChars      int
	maxTokens     int
	client        *http.Client
	questionWords []string
	addressWords  []string
	shareWords    []string
	systemPrompt  string
}

// NewResponseGate returns nil when the gate should be inactive
// (disabled in config or no API key available).
func NewResponseGate(
	cfg config.RealtimeGate,
	fastpath config.GateFastpath,
	systemPrompt, apiKey, apiBase string,
) *ResponseGate {
	if !cfg.Enabled || apiKey == "" {
		return nil
	}
	model := cfg.Model
	timeoutMS := cfg.TimeoutMS
	maxChars := cfg.MaxChars
	maxTokens := cfg.MaxTokens
	base := strings.TrimRight(apiBase, "/")
	if base == "" {
		base = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return &ResponseGate{
		apiKey:        apiKey,
		apiBase:       base,
		model:         model,
		timeout:       time.Duration(timeoutMS) * time.Millisecond,
		maxChars:      maxChars,
		maxTokens:     maxTokens,
		client:        &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
		questionWords: fastpath.QuestionWords,
		addressWords:  fastpath.AddressWords,
		shareWords:    fastpath.ShareWords,
		systemPrompt:  systemPrompt,
	}
}

// Decide reports whether the turn should proceed to the LLM, plus a short
// reason for logging. Fail-open: errors yield ok=true.
func (g *ResponseGate) Decide(ctx context.Context, text, petName string) (bool, string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return false, "empty"
	}

	if strings.ContainsAny(t, "?？") {
		return true, "fastpath:question_mark"
	}
	for _, w := range g.questionWords {
		if strings.Contains(t, w) {
			return true, "fastpath:question_word:" + w
		}
	}
	if petName != "" && strings.Contains(t, petName) {
		return true, "fastpath:pet_name"
	}
	for _, w := range g.shareWords {
		if strings.Contains(t, w) {
			return true, "fastpath:share:" + w
		}
	}
	for _, w := range g.addressWords {
		if strings.Contains(t, w) {
			return true, "fastpath:address:" + w
		}
	}
	// Phase C：信息类（天气/新闻等）零延迟放行 + 后续 LLM 并行联网搜索
	if agent.NeedsWebSearch(t) {
		return true, "fastpath:info_query"
	}

	runes := []rune(t)
	if len(runes) > g.maxChars {
		t = string(runes[:g.maxChars])
	}

	respond, err := g.askModel(ctx, t, petName)
	if err != nil {
		return true, "failopen:" + err.Error()
	}
	if !respond {
		return false, "llm:respond=false"
	}
	return true, "llm:respond=true"
}

type gateChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type gateChatRequest struct {
	Model       string            `json:"model"`
	Messages    []gateChatMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
}

type gateChatResponse struct {
	Choices []struct {
		Message gateChatMessage `json:"message"`
	} `json:"choices"`
}

func (g *ResponseGate) askModel(ctx context.Context, text, petName string) (bool, error) {
	userMsg := text
	if petName != "" {
		userMsg = fmt.Sprintf("（桌面宠物名字叫%s）%s", petName, text)
	}
	reqBody := gateChatRequest{
		Model: g.model,
		Messages: []gateChatMessage{
			{Role: "system", Content: g.systemPrompt},
			{Role: "user", Content: userMsg},
		},
		Temperature: 0,
		MaxTokens:   g.maxTokens,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return true, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		g.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return true, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return true, err
	}
	if resp.StatusCode != http.StatusOK {
		return true, fmt.Errorf("gate llm status=%d", resp.StatusCode)
	}

	var chatResp gateChatResponse
	if err := json.Unmarshal(respData, &chatResp); err != nil {
		return true, err
	}
	if len(chatResp.Choices) == 0 {
		return true, fmt.Errorf("gate llm empty choices")
	}
	return parseGateAnswer(chatResp.Choices[0].Message.Content)
}

// parseGateAnswer extracts the respond flag from the model output.
// Fail-open: unparseable output yields respond=true.
func parseGateAnswer(content string) (bool, error) {
	c := strings.TrimSpace(content)
	c = strings.TrimPrefix(c, "```json")
	c = strings.TrimPrefix(c, "```")
	c = strings.TrimSuffix(c, "```")
	c = strings.TrimSpace(c)

	start := strings.Index(c, "{")
	end := strings.LastIndex(c, "}")
	if start < 0 || end <= start {
		return true, fmt.Errorf("gate llm no json: %q", content)
	}
	var out struct {
		Respond bool `json:"respond"`
	}
	if err := json.Unmarshal([]byte(c[start:end+1]), &out); err != nil {
		return true, err
	}
	return out.Respond, nil
}
