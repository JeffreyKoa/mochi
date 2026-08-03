package vision

import (
	"strings"
)

// Tier0PauseHint 句中 pause_probe 的 Tier-0 推断（无云端 VL）。
func Tier0PauseHint(partialText string, face *FaceProbeIn) (expression string, composing bool) {
	partialText = strings.TrimSpace(partialText)
	unfinished := tier0PartialUnfinished(partialText)

	if face != nil && face.Detected {
		if unfinished || partialText == "" {
			return "thinking", true
		}
		if face.Score >= 0.45 && len([]rune(partialText)) < 16 {
			return "hesitant", true
		}
	}
	if unfinished {
		return "thinking", true
	}
	return "neutral", false
}

// Tier0GlanceHint THINK 阶段 Tier-0 微探（仅本地 face_probe，禁止 Tier-1）。
func Tier0GlanceHint(face *FaceProbeIn) Hint {
	if face == nil || !face.Detected {
		return EmptyHint()
	}
	expr := "neutral"
	if face.Score < 0.4 {
		expr = "thinking"
	}
	return Hint{
		Focus:                FocusOwnerFace,
		UserExpression:       expr,
		ExpressionConfidence: face.Score,
		Note:                 "tier0_glance",
	}
}

// FaceProbeIn 与 realtime.VisionFrameIn 解耦的 Tier-0 输入。
type FaceProbeIn struct {
	Match    bool
	Score    float64
	Detected bool
}

func tier0PartialUnfinished(text string) bool {
	if text == "" {
		return false
	}
	connectives := []string{
		"但是", "因为", "所以", "然后", "而且", "如果", "不过", "虽然", "觉得", "比如",
		"或者", "就是", "也就是说", "然后呢", "……", "...",
	}
	for _, c := range connectives {
		if strings.HasSuffix(text, c) {
			return true
		}
	}
	last, _ := utf8LastRune(text)
	if last == '，' || last == ',' || last == '：' || last == ':' {
		return true
	}
	rs := []rune(text)
	if len(rs) < 8 && !strings.ContainsAny(text, "。！？?!") {
		return true
	}
	return false
}

func utf8LastRune(s string) (rune, bool) {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) == 0 {
		return 0, false
	}
	return rs[len(rs)-1], true
}
