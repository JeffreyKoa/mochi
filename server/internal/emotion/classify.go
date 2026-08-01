package emotion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/vision"
	"github.com/mochi-ai/server/pkg/ai"
)

// classifyJSON LLM 分类输出 schema。
type classifyJSON struct {
	UserMood      string `json:"user_mood"`
	Intent        string `json:"intent"`
	NeedsEmpathy  bool   `json:"needs_empathy"`
	Topic         string `json:"topic"`
	VisualTask    string `json:"visual_task"`
	FaceTextClash bool   `json:"face_text_clash"`
	Reason        string `json:"reason"`
}

// ClassifyUtterance 同步轻量语义分类：主人原话 + SER + VL 读数（V3c，不用关键词表）。
func (s *Service) ClassifyUtterance(ctx context.Context, text string, acoustic AcousticHint, face vision.Hint) UtteranceInsight {
	if s == nil || !s.classifyEnabled || s.ai == nil {
		return FallbackInsight(acoustic, face)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return FallbackInsight(acoustic, face)
	}

	prompt := buildClassifyPrompt(text, acoustic, face)
	reqCtx := ctx
	cancel := func() {}
	if s.classifyTimeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, s.classifyTimeout)
	}
	defer cancel()

	start := time.Now()
	resp, err := s.ai.Chat(reqCtx, ai.ChatRequest{
		Model:       s.classifyModel,
		Messages:    []ai.Message{{Role: "user", Content: prompt}},
		Temperature: 0.1,
		MaxTokens:   180,
	})
	if err != nil {
		log.Printf("[emotion][classify] error err=%v elapsed_ms=%d", err, time.Since(start).Milliseconds())
		return FallbackInsight(acoustic, face)
	}

	ins := parseClassifyResponse(resp.Content)
	log.Printf("[emotion][classify] mood=%s intent=%s visual_task=%s clash=%v reason=%q elapsed_ms=%d",
		ins.UserMood, ins.Intent, ins.VisualTask, ins.FaceTextClash, ins.Reason, time.Since(start).Milliseconds())
	return ins
}

func buildClassifyPrompt(text string, acoustic AcousticHint, face vision.Hint) string {
	acousticLine := "声学：未识别"
	if acoustic.Confidence > 0 {
		acousticLine = fmt.Sprintf("声学情绪=%s 置信度=%.2f", acoustic.Mood, acoustic.Confidence)
	}
	faceLine := "视觉：无画面或未分析"
	if face.Focus == vision.FocusOwnerFace && !face.Skipped {
		faceLine = fmt.Sprintf("面部表情=%s 置信度=%.2f 描述=%q",
			face.UserExpression, face.ExpressionConfidence, face.Note)
	}

	return fmt.Sprintf(`你是语音陪伴产品的感知模块。根据主人刚说的话、声学情绪和面部表情读数，判断其状态。
不要依赖固定关键词表；从语义与多模态信号综合推断。

主人说：「%s」
%s
%s

返回 JSON（仅 JSON，无其他文字）：
{"user_mood":"stressed|sad|happy|angry|neutral|excited","intent":"vent|chat|ask|plan|joke","needs_empathy":true/false,"topic":"工作|感情|健康|娱乐|生活|其他","visual_task":"none|object|scene|face_consistency","face_text_clash":true/false,"reason":"一句话"}

规则提示：
- visual_task=object：主人在问/让你看某物品（如「这是什么」「手里拿的啥」）
- visual_task=scene：主人让你看环境/窗外/房间
- visual_task=face_consistency：话与脸/语气可能不一致（如说「没事」但听起来或看起来不好）
- face_text_clash：文字偏 neutral/positive 但 SER 或表情偏负面，或明显掩饰
- needs_empathy：需要宠物共情安慰时为 true`, text, acousticLine, faceLine)
}

func parseClassifyResponse(content string) UtteranceInsight {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var j classifyJSON
	if err := json.Unmarshal([]byte(content), &j); err != nil {
		log.Printf("[emotion][classify] json_fail err=%v raw=%q", err, truncateClassifyRaw(content, 120))
		return UtteranceInsight{UserMood: "neutral", Intent: "chat", VisualTask: VisualTaskNone}
	}
	ins := UtteranceInsight{
		UserMood:      strings.ToLower(strings.TrimSpace(j.UserMood)),
		Intent:        strings.ToLower(strings.TrimSpace(j.Intent)),
		NeedsEmpathy:  j.NeedsEmpathy,
		Topic:         strings.TrimSpace(j.Topic),
		VisualTask:    NormalizeVisualTask(j.VisualTask),
		FaceTextClash: j.FaceTextClash,
		Reason:        strings.TrimSpace(j.Reason),
	}
	if ins.UserMood == "" {
		ins.UserMood = "neutral"
	}
	if ins.Intent == "" {
		ins.Intent = "chat"
	}
	if ins.Intent == "vent" {
		ins.NeedsEmpathy = true
	}
	return ins
}

// FallbackInsight 分类不可用时的降级：仅 SER + VL 读数，不做文本关键词匹配。
func FallbackInsight(acoustic AcousticHint, face vision.Hint) UtteranceInsight {
	ins := UtteranceInsight{
		UserMood:   "neutral",
		Intent:     "chat",
		VisualTask: VisualTaskNone,
		Reason:     "classify_fallback",
	}
	if acoustic.Confidence >= 0.65 {
		switch strings.ToLower(strings.TrimSpace(acoustic.Mood)) {
		case "sad", "stressed":
			ins.UserMood = strings.ToLower(acoustic.Mood)
			ins.NeedsEmpathy = true
			ins.Intent = "vent"
		case "happy":
			ins.UserMood = "happy"
			ins.Intent = "joke"
		}
	}
	if face.Focus == vision.FocusOwnerFace && face.ExpressionConfidence >= 0.6 {
		expr := strings.ToLower(face.UserExpression)
		if (expr == "sad" || expr == "tired" || expr == "anxious") && ins.UserMood == "neutral" {
			ins.NeedsEmpathy = true
			if ins.Intent == "chat" {
				ins.Intent = "vent"
			}
			if mood := expressionToMood(expr); mood != "" {
				ins.UserMood = mood
			}
		}
		if expr == "happy" && ins.UserMood == "neutral" {
			ins.UserMood = "happy"
		}
	}
	return ins
}

func truncateClassifyRaw(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
