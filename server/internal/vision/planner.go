package vision

import (
	"log"
	"strings"
)

// RefinePlan ASR 后视觉 refine 决策（V3a 关键词 / V3c 动态 prompt）。
type RefinePlan struct {
	NeedSecondVL  bool
	Focus         Focus
	Reason        string
	DynamicPrompt string // V3c：上下文 VL 用户 prompt
}

// PlanContextual V3c：由语义分类 visual_task 驱动二次 VL，不依赖关键词表。
func PlanContextual(userText string, faceHint Hint, visualTask string, faceTextClash bool) RefinePlan {
	text := strings.TrimSpace(userText)
	if text == "" {
		return RefinePlan{NeedSecondVL: false, Focus: FocusOwnerFace, Reason: "empty_text"}
	}
	task := strings.ToLower(strings.TrimSpace(visualTask))
	switch task {
	case "object":
		log.Printf("[vision][planner] contextual task=object text=%q", truncate(text, 40))
		return RefinePlan{
			NeedSecondVL:  true,
			Focus:         FocusObject,
			DynamicPrompt: PromptContextualObject(text),
			Reason:        "contextual:visual_task_object",
		}
	case "scene":
		log.Printf("[vision][planner] contextual task=scene text=%q", truncate(text, 40))
		return RefinePlan{
			NeedSecondVL:  true,
			Focus:         FocusScene,
			DynamicPrompt: PromptContextualScene(text),
			Reason:        "contextual:visual_task_scene",
		}
	case "face_consistency":
		if faceTextClash || faceHint.IsUsable() {
			log.Printf("[vision][planner] contextual task=face_consistency text=%q expression=%s",
				truncate(text, 40), faceHint.UserExpression)
			return RefinePlan{
				NeedSecondVL:  true,
				Focus:         FocusOwnerFace,
				DynamicPrompt: PromptContextualFaceConsistency(text, faceHint),
				Reason:        "contextual:face_text_clash",
			}
		}
	}
	if faceTextClash && faceHint.IsUsable() {
		return RefinePlan{
			NeedSecondVL:  true,
			Focus:         FocusOwnerFace,
			DynamicPrompt: PromptContextualFaceConsistency(text, faceHint),
			Reason:        "contextual:face_clash_flag",
		}
	}
	log.Printf("[vision][planner] contextual skip text=%q reason=owner_face_sufficient", truncate(text, 40))
	return RefinePlan{NeedSecondVL: false, Focus: FocusOwnerFace, Reason: "contextual:owner_face_sufficient"}
}

// PlanRefine 根据 ASR 文本决定是否需要二次 VL（object/scene）。
func PlanRefine(userText string, objectKeys, sceneKeys []string) RefinePlan {
	text := strings.TrimSpace(userText)
	if text == "" {
		return RefinePlan{NeedSecondVL: false, Focus: FocusOwnerFace, Reason: "empty_text"}
	}
	lower := strings.ToLower(text)
	for _, kw := range objectKeys {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
			log.Printf("[vision][planner] refine=object trigger=%q text=%q", kw, truncate(text, 40))
			return RefinePlan{NeedSecondVL: true, Focus: FocusObject, Reason: "object_trigger:" + kw}
		}
	}
	for _, kw := range sceneKeys {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
			log.Printf("[vision][planner] refine=scene trigger=%q text=%q", kw, truncate(text, 40))
			return RefinePlan{NeedSecondVL: true, Focus: FocusScene, Reason: "scene_trigger:" + kw}
		}
	}
	log.Printf("[vision][planner] refine=skip text=%q reason=owner_face_sufficient", truncate(text, 40))
	return RefinePlan{NeedSecondVL: false, Focus: FocusOwnerFace, Reason: "owner_face_sufficient"}
}
