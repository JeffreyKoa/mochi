package vision

import "strings"

// isVLTemplateGarbage 过滤 Moondream 复读 prompt 占位符或仅输出品牌名的无效结果。
func isVLTemplateGarbage(raw string) bool {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"`)
	if s == "" {
		return false
	}
	switch strings.ToLower(s) {
	case "mochi", "mochi 第一人称中文", "第一人称中文":
		return true
	}
	if strings.Contains(s, "第一人称中文") && len([]rune(s)) <= 16 {
		return true
	}
	if strings.EqualFold(s, "Mochi") {
		return true
	}
	return false
}

// markVLGarbage 将无效 VL 输出标记为 skipped，避免 merge_ok / visual_in_prompt。
func markVLGarbage(h Hint, reason string) Hint {
	h.Skipped = true
	h.SkipReason = reason
	h.ExpressionConfidence = 0
	if h.UserExpression == "" {
		h.UserExpression = "unknown"
	}
	return h
}

// WantsHardFocusVision 用户句是否需 object/scene 硬焦点视觉（应跳过 owner_face 并行）。
func WantsHardFocusVision(userText string, objectKeys, sceneKeys []string) bool {
	switch InferVisualTaskFromText(userText) {
	case "object", "scene":
		return true
	}
	return PlanNeedsHardBarrier(PlanRefine(userText, objectKeys, sceneKeys))
}
