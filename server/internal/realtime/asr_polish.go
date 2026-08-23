package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/modelmeta"
)

// PolishContext 供 ASR 润色使用的会话上下文。
type PolishContext struct {
	SessionID string
	PetName   string
}

// PolishResult ASR 润色结果（fail-open：失败时 Polished=Original）。
type PolishResult struct {
	Original string
	Polished string
	Changed  bool
	Reason   string // rules | llm | skipped | unchanged
}

// ASRPolisher 对 ASR final 做轻量纠错（规则 + 可选 LLM）。
type ASRPolisher struct {
	cfg         config.RealtimeASRPolish
	homophones  config.ASRPolishHomophones
	apiKey      string
	apiBase     string
	defaultModel string
	fallbackModel string
	client      *http.Client
}

// NewASRPolisher 创建润色器；cfg.Enabled=false 时 Enabled() 为 false。
func NewASRPolisher(
	cfg config.RealtimeASRPolish,
	homophones config.ASRPolishHomophones,
	apiKey, apiBase, defaultModel, fallbackModel string,
) *ASRPolisher {
	timeout := time.Duration(cfg.LLMTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	return &ASRPolisher{
		cfg:           cfg,
		homophones:    homophones,
		apiKey:        strings.TrimSpace(apiKey),
		apiBase:       strings.TrimRight(strings.TrimSpace(apiBase), "/"),
		defaultModel:  strings.TrimSpace(defaultModel),
		fallbackModel: strings.TrimSpace(fallbackModel),
		client:        &http.Client{Timeout: timeout},
	}
}

func (p *ASRPolisher) Enabled() bool {
	return p != nil && p.cfg.Enabled
}

// Polish 润色 ASR 文本；任何失败返回原文。
func (p *ASRPolisher) Polish(ctx context.Context, text string, pc PolishContext) PolishResult {
	raw := strings.TrimSpace(text)
	if !p.Enabled() || raw == "" {
		return PolishResult{Original: raw, Polished: raw, Reason: "skipped"}
	}

	out := rulesPolish(raw, pc.PetName, p.homophones)
	reason := "unchanged"
	if out != raw {
		reason = "rules"
	}

	mode := strings.ToLower(strings.TrimSpace(p.cfg.Mode))
	if mode == "" {
		mode = "rules"
	}

	tryLLM := mode == "llm" || (mode == "rules_then_llm" && out == raw && shouldTryLLMPolish(out))
	if tryLLM && p.apiKey != "" && p.apiBase != "" {
		if p.cfg.MaxChars > 0 && len([]rune(out)) > p.cfg.MaxChars {
			tryLLM = false
		}
	}
	if tryLLM {
		llmOut, ok := p.llmPolish(ctx, out, pc.PetName)
		if ok && strings.TrimSpace(llmOut) != "" && llmOut != out {
			if polishChangeAcceptable(out, llmOut) {
				out = strings.TrimSpace(llmOut)
				reason = "llm"
			}
		}
	}

	return PolishResult{
		Original: raw,
		Polished: out,
		Changed:  out != raw,
		Reason:   reason,
	}
}

func rulesPolish(text, petName string, h config.ASRPolishHomophones) string {
	out := text
	for _, pair := range h.Replacements {
		from := strings.TrimSpace(pair.From)
		to := pair.To
		if from == "" {
			continue
		}
		out = strings.ReplaceAll(out, from, to)
	}
	if petName != "" {
		for _, alias := range h.PetNameAliases {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				out = strings.ReplaceAll(out, alias, petName)
			}
		}
	}
	for _, filler := range h.StripFillers {
		filler = strings.TrimSpace(filler)
		if filler == "" {
			continue
		}
		out = stripLeadingFiller(out, filler)
	}
	return strings.TrimSpace(out)
}

func stripLeadingFiller(text, filler string) string {
	t := strings.TrimSpace(text)
	for strings.HasPrefix(t, filler) {
		t = strings.TrimSpace(strings.TrimPrefix(t, filler))
	}
	return t
}

func shouldTryLLMPolish(text string) bool {
	for _, r := range text {
		if unicode.IsDigit(r) {
			return true
		}
	}
	keywords := []string{"分钟", "小时", "提醒", "待办", "明天", "今天", "名字", "叫我"}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// polishChangeAcceptable 改动过大则丢弃 LLM 结果，避免改义。
func polishChangeAcceptable(before, after string) bool {
	br := []rune(before)
	ar := []rune(after)
	if len(br) == 0 {
		return len(ar) > 0
	}
	diff := len(ar) - len(br)
	if diff < 0 {
		diff = -diff
	}
	if diff > len(br)/2+8 {
		return false
	}
	return true
}

type polishChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type polishChatRequest struct {
	Model       string              `json:"model"`
	Messages    []polishChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
}

type polishChatResponse struct {
	Choices []struct {
		Message polishChatMessage `json:"message"`
	} `json:"choices"`
}

func (p *ASRPolisher) llmPolish(ctx context.Context, text, petName string) (string, bool) {
	model := strings.TrimSpace(p.cfg.LLMModel)
	if model == "" {
		model = p.fallbackModel
	}
	if model == "" {
		model = p.defaultModel
	}
	if model == "" {
		return text, false
	}

	modelmeta.LogCall("asr_polish", modelmeta.InferVendorFromAPIBase(p.apiBase), model)
	system := "你是语音识别后处理。只输出修正后的用户原话一句，不要解释、不要引号、不要 markdown。保留原意；专有名词用给定宠物名。"
	user := text
	if petName != "" {
		user = fmt.Sprintf("（宠物名：%s）%s", petName, text)
	}

	reqBody := polishChatRequest{
		Model: model,
		Messages: []polishChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
		MaxTokens:   128,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return text, false
	}

	timeout := time.Duration(p.cfg.LLMTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return text, false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	start := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		log.Printf("[asr_polish] llm error: %v", err)
		return text, false
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[asr_polish] llm http %d ms=%d", resp.StatusCode, time.Since(start).Milliseconds())
		return text, false
	}

	var parsed polishChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return text, false
	}
	out := strings.TrimSpace(parsed.Choices[0].Message.Content)
	out = strings.Trim(out, `"「」"'`)
	log.Printf("[asr_polish] llm ms=%d out=%q", time.Since(start).Milliseconds(), out)
	if out == "" {
		return text, false
	}
	return out, true
}
