package emotion

import (
	"strings"

	"github.com/mochi-ai/server/internal/vision"
)

// PerceptionState Pipeline 单 turn 融合后的唯一感知真相源（V3c）。
type PerceptionState struct {
	Hint     Hint
	Insight  UtteranceInsight
	Visual   vision.Hint
	Acoustic AcousticHint
	Text     string
	Source   string
}

// BuildFinalPerception 融合 Insight + SER + VL，供 Pipeline 与 Agent 共用。
func BuildFinalPerception(
	text string,
	acoustic AcousticHint,
	visual vision.Hint,
	insight UtteranceInsight,
	minAcoustic, minVisual float64,
) PerceptionState {
	if minVisual <= 0 {
		minVisual = 0.6
	}
	base := HintFromInsight(insight)
	merged := MergeAcousticHint(Hint{}, base, acoustic, minAcoustic)
	merged = MergeVisualHint(merged, visual, minVisual)
	merged = applyInsightClash(merged, insight, visual, minVisual)

	if visual.IsUsable() {
		if merged.VisualNote == "" && visual.Note != "" {
			merged.VisualNote = visual.Note
		}
		if merged.VisualFocus == "" && visual.Focus != vision.FocusSkip {
			merged.VisualFocus = string(visual.Focus)
		}
	}

	return PerceptionState{
		Hint:     merged,
		Insight:  insight,
		Visual:   visual,
		Acoustic: acoustic,
		Text:     strings.TrimSpace(text),
		Source:   "pipeline_v3c",
	}
}

// applyInsightClash 分类器标记的话脸不一致 → 共情加深（数据驱动，非关键词）。
func applyInsightClash(h Hint, ins UtteranceInsight, v vision.Hint, minVisual float64) Hint {
	if !ins.FaceTextClash {
		return h
	}
	if v.Focus == vision.FocusOwnerFace && v.ExpressionConfidence >= minVisual {
		if h.VisualNote == "" && v.Note != "" {
			h.VisualNote = v.Note
		}
		h.VisualFocus = string(vision.FocusOwnerFace)
	}
	if !h.NeedsEmpathy {
		h.NeedsEmpathy = true
		if h.Intent == "chat" {
			h.Intent = "vent"
		}
		if h.UserMood == "" || h.UserMood == "neutral" {
			if mood := expressionToMood(strings.ToLower(v.UserExpression)); mood != "" {
				h.UserMood = mood
			} else {
				h.UserMood = "stressed"
			}
		}
		h.Temperature = 0.75
	}
	return h
}
