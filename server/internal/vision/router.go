package vision

import (
	"log"
	"strings"
)

// RouteInput 注意力路由输入。
type RouteInput struct {
	UserText      string
	Intent        string
	IsVoiceTurn   bool
	VisionEnabled bool
	HasFrame      bool
	ObjectKeys    []string
	SceneKeys     []string
	SkipTopics    []string
	DeicticExempt bool
}

// RouteFocus 根据谈话上下文选择视觉焦点（V1.5：object > scene > owner_face > skip）。
func RouteFocus(in RouteInput) Focus {
	text := strings.TrimSpace(in.UserText)
	if !in.VisionEnabled {
		log.Printf("[vision][router] focus=skip reason=vision_disabled text=%q", truncate(text, 40))
		return FocusSkip
	}
	if !in.HasFrame {
		log.Printf("[vision][router] focus=skip reason=no_frame text=%q voice=%v", truncate(text, 40), in.IsVoiceTurn)
		return FocusSkip
	}
	if !in.IsVoiceTurn {
		log.Printf("[vision][router] focus=skip reason=not_voice_turn text=%q", truncate(text, 40))
		return FocusSkip
	}
	// Step1：纯信息意图 Skip Tier-1（代词豁免见 skip.go）
	if ShouldSkipTier1(text, in.SkipTopics, in.DeicticExempt) {
		log.Printf("[vision][router] focus=skip reason=skip_topic text=%q", truncate(text, 40))
		return FocusSkip
	}

	lower := strings.ToLower(text)
	for _, kw := range in.ObjectKeys {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
			log.Printf("[vision][router] focus=object trigger=%q text=%q intent=%q", kw, truncate(text, 40), in.Intent)
			return FocusObject
		}
	}
	for _, kw := range in.SceneKeys {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
			log.Printf("[vision][router] focus=scene trigger=%q text=%q intent=%q", kw, truncate(text, 40), in.Intent)
			return FocusScene
		}
	}

	log.Printf("[vision][router] focus=owner_face intent=%q text=%q", in.Intent, truncate(text, 40))
	return FocusOwnerFace
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
