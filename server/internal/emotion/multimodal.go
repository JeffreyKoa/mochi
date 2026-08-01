package emotion

import (
	"log"
	"strings"

	"github.com/mochi-ai/server/internal/vision"
)

// applyVisualEmpathy owner_face 高置信表情 → 与声学叠加的共情信号（vent 文本仍优先）。
func applyVisualEmpathy(h Hint, v vision.Hint, minConf float64) Hint {
	if v.Focus != vision.FocusOwnerFace || v.ExpressionConfidence < minConf {
		return h
	}

	expr := strings.ToLower(strings.TrimSpace(v.UserExpression))
	moodFromVisual := expressionToMood(expr)
	if moodFromVisual == "" {
		return h
	}

	// vent 关键词已由 MergeAcousticHint 锁定，仅补充 VisualNote
	if h.NeedsEmpathy && h.Intent == "vent" {
		if h.VisualNote == "" && v.Note != "" {
			h.VisualNote = v.Note
			h.VisualFocus = string(vision.FocusOwnerFace)
		}
		log.Printf("[emotion][multimodal] visual_skip_mood reason=vent_text_wins expression=%s", expr)
		return h
	}

	textMood := strings.ToLower(strings.TrimSpace(h.UserMood))
	acousticEmpathy := h.NeedsEmpathy

	// 强颜欢笑：文字 happy + 脸 sad/anxious
	if (textMood == "happy" || h.Intent == "joke") && (expr == "sad" || expr == "anxious") {
		if h.VisualNote == "" {
			if v.Note != "" {
				h.VisualNote = v.Note
			} else {
				h.VisualNote = "主人可能在强颜欢笑"
			}
		}
		h.VisualFocus = string(vision.FocusOwnerFace)
		if !acousticEmpathy {
			h.UserMood = "stressed"
			h.NeedsEmpathy = true
			if h.Intent == "chat" || h.Intent == "joke" {
				h.Intent = "vent"
			}
			h.Temperature = 0.75
		}
		log.Printf("[emotion][multimodal] forced_smile visual=%s acoustic_empathy=%v mood=%s intent=%s",
			expr, acousticEmpathy, h.UserMood, h.Intent)
		return h
	}

	// 声学已共情：保留声学 mood，只 enrich note
	if acousticEmpathy {
		if h.VisualNote == "" && v.Note != "" {
			h.VisualNote = v.Note
			h.VisualFocus = string(vision.FocusOwnerFace)
		}
		log.Printf("[emotion][multimodal] stack_visual_on_acoustic expression=%s acoustic_mood=%s note=%q",
			expr, h.UserMood, h.VisualNote)
		return h
	}

	// neutral 文字 + 负面表情 → 视觉共情
	if textMood == "" || textMood == "neutral" {
		h.UserMood = moodFromVisual
		h.NeedsEmpathy = true
		if h.Intent == "chat" {
			h.Intent = "vent"
		}
		h.Temperature = 0.75
		if h.VisualNote == "" && v.Note != "" {
			h.VisualNote = v.Note
		}
		h.VisualFocus = string(vision.FocusOwnerFace)
		log.Printf("[emotion][multimodal] visual_empathy expression=%s conf=%.2f mood=%s intent=%s",
			expr, v.ExpressionConfidence, h.UserMood, h.Intent)
	}

	return h
}

func expressionToMood(expr string) string {
	switch expr {
	case "sad":
		return "sad"
	case "anxious":
		return "stressed"
	case "tired":
		return "stressed"
	default:
		return ""
	}
}
