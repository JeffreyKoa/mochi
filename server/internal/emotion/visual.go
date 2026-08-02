package emotion

import (
	"log"
	"strings"

	"github.com/mochi-ai/server/internal/vision"
)

// MergeVisualHint 将视觉 Hint 合并进情绪 Hint。
// owner_face：高置信表情 → VisualNote（可影响共情 Prompt，不直接改 FSM）。
// object/scene：仅 VisualNote + VisualFocus，**不**覆写 UserMood/Intent。
func MergeVisualHint(h Hint, v vision.Hint, minConf float64) Hint {
	if v.Skipped || v.Focus == vision.FocusSkip || !v.IsUsable() {
		log.Printf("[emotion][visual] merge_skip skipped=%v focus=%s reason=%q conf=%.2f min=%.2f",
			v.Skipped, v.Focus, v.SkipReason, v.ExpressionConfidence, minConf)
		return h
	}

	switch v.Focus {
	case vision.FocusOwnerFace:
		if v.ExpressionConfidence < minConf {
			log.Printf("[emotion][visual] merge_skip low_conf expression=%s conf=%.2f min=%.2f note=%q",
				v.UserExpression, v.ExpressionConfidence, minConf, v.Note)
			return h
		}
		if h.VisualNote == "" && v.Note != "" {
			h.VisualNote = vision.SanitizeCompanionNote(v.Note)
			h.VisualFocus = string(vision.FocusOwnerFace)
		}
		h = applyVisualEmpathy(h, v, minConf)
		log.Printf("[emotion][visual] merge_ok focus=owner_face expression=%s conf=%.2f note=%q mood=%s intent=%s empathy=%v",
			v.UserExpression, v.ExpressionConfidence, h.VisualNote, h.UserMood, h.Intent, h.NeedsEmpathy)

	case vision.FocusObject:
		note := strings.TrimSpace(v.Note)
		if note == "" {
			note = strings.TrimSpace(v.ObjectSummary)
		}
		if note == "" {
			log.Printf("[emotion][visual] merge_skip focus=object empty_note")
			return h
		}
		note = vision.SanitizeCompanionNote(note)
		if summary := strings.TrimSpace(v.ObjectSummary); summary != "" {
			summary = vision.SanitizeCompanionNote(summary)
			if summary == "没看清楚" && !strings.Contains(note, "没看清楚") {
				note = "我没看清楚：" + note
			}
		}
		log.Printf("[emotion][visual] merge_ok focus=object note=%q mood_unchanged=%s intent_unchanged=%s",
			note, h.UserMood, h.Intent)
		h.VisualNote = note
		h.VisualFocus = string(vision.FocusObject)

	case vision.FocusScene:
		note := strings.TrimSpace(v.Note)
		if note == "" {
			note = strings.TrimSpace(v.SceneSummary)
		}
		if note == "" {
			log.Printf("[emotion][visual] merge_skip focus=scene empty_note")
			return h
		}
		note = vision.SanitizeCompanionNote(note)
		log.Printf("[emotion][visual] merge_ok focus=scene note=%q mood_unchanged=%s intent_unchanged=%s",
			note, h.UserMood, h.Intent)
		h.VisualNote = note
		h.VisualFocus = string(vision.FocusScene)
	}

	return h
}
