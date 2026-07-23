package emotion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

const contextKeyPrefix = "mochi:emotion:ctx:"

type Hint struct {
	UserMood      string `json:"user_mood"`
	Intent        string `json:"intent"`
	NeedsEmpathy  bool   `json:"needs_empathy"`
	Topic         string `json:"topic"`
	Temperature   float64
}

type Service struct {
	rdb *redis.Client
	ai  *ai.Provider
}

func NewService(rdb *redis.Client, aiProvider *ai.Provider) *Service {
	return &Service{rdb: rdb, ai: aiProvider}
}

var ventKeywords = []string{
	"烦", "累", "崩溃", "骂", "委屈", "难过", "伤心", "焦虑", "压力", "抑郁",
	"受不了", "不想干", "好烦", "烦死", "气死", "破防", "emo", "烦透了",
}

func QuickDetect(message string) Hint {
	h := Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
	msg := strings.ToLower(message)
	for _, kw := range ventKeywords {
		if strings.Contains(msg, kw) {
			h.UserMood = "stressed"
			h.Intent = "vent"
			h.NeedsEmpathy = true
			h.Temperature = 0.75
			return h
		}
	}
	if strings.Contains(msg, "哈哈") || strings.Contains(msg, "笑") || strings.Contains(msg, "梗") {
		h.UserMood = "happy"
		h.Intent = "joke"
		h.Temperature = 0.9
	}
	if strings.Contains(msg, "提醒") || strings.Contains(msg, "记得") || strings.Contains(msg, "明天") {
		h.Intent = "plan"
	}
	if strings.Contains(msg, "？") || strings.Contains(msg, "?") || strings.Contains(msg, "怎么") || strings.Contains(msg, "什么") {
		if h.Intent == "chat" {
			h.Intent = "ask"
		}
	}
	return h
}

func (s *Service) GetCached(ctx context.Context, petID uint64) Hint {
	key := fmt.Sprintf("%s%d", contextKeyPrefix, petID)
	data, err := s.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
	}
	var h Hint
	if json.Unmarshal(data, &h) == nil {
		if h.Temperature == 0 {
			h.Temperature = 0.85
		}
		return h
	}
	return Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
}

func (s *Service) Cache(ctx context.Context, petID uint64, h Hint) error {
	key := fmt.Sprintf("%s%d", contextKeyPrefix, petID)
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, data, 30*time.Minute).Err()
}

func (s *Service) ClassifyAsync(ctx context.Context, petID uint64, userMsg, petReply string, history []models.ChatMessage) {
	if s.ai == nil {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hint := s.classify(bgCtx, userMsg, petReply, history)
		_ = s.Cache(bgCtx, petID, hint)
	}()
}

func (s *Service) classify(ctx context.Context, userMsg, petReply string, history []models.ChatMessage) Hint {
	var sb strings.Builder
	start := len(history) - 6
	if start < 0 {
		start = 0
	}
	for _, m := range history[start:] {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	sb.WriteString(fmt.Sprintf("user: %s\nassistant: %s", userMsg, petReply))

	prompt := fmt.Sprintf(`分析以下对话中用户的状态，返回 JSON（仅 JSON，无其他文字）:
{"user_mood":"stressed|sad|happy|angry|neutral|excited","intent":"vent|chat|ask|plan|joke","needs_empathy":true/false,"topic":"工作|感情|健康|娱乐|生活|其他"}

对话:
%s`, sb.String())

	resp, err := s.ai.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: "user", Content: prompt}},
		Temperature: 0.1,
		MaxTokens:   120,
	})
	if err != nil {
		return QuickDetect(userMsg)
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var h Hint
	if err := json.Unmarshal([]byte(content), &h); err != nil {
		return QuickDetect(userMsg)
	}
	h.Temperature = 0.85
	if h.NeedsEmpathy || h.Intent == "vent" {
		h.Temperature = 0.75
	}
	if h.Intent == "joke" {
		h.Temperature = 0.9
	}
	if h.UserMood == "" {
		h.UserMood = "neutral"
	}
	if h.Intent == "" {
		h.Intent = "chat"
	}
	return h
}

func MergeHint(cached, quick Hint, currentMsg string) Hint {
	if quick.NeedsEmpathy || quick.Intent == "vent" {
		return quick
	}
	if cached.UserMood != "" && cached.UserMood != "neutral" {
		merged := cached
		if quick.Intent != "chat" {
			merged.Intent = quick.Intent
		}
		if quick.Temperature != 0 {
			merged.Temperature = quick.Temperature
		}
		return merged
	}
	return quick
}

func IsNegativeMood(mood string) bool {
	switch mood {
	case "stressed", "sad", "angry":
		return true
	default:
		return false
	}
}
