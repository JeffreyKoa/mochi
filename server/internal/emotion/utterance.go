package emotion

import "strings"

// VisualTask ASR+多模态分类器建议的视觉二次分析任务（V3c，非关键词）。
const (
	VisualTaskNone            = "none"
	VisualTaskObject          = "object"
	VisualTaskScene           = "scene"
	VisualTaskFaceConsistency = "face_consistency"
)

// UtteranceInsight 轻量语义分类结果：来自主人原话 + SER + VL 初步读数。
type UtteranceInsight struct {
	UserMood      string `json:"user_mood"`
	Intent        string `json:"intent"`
	NeedsEmpathy  bool   `json:"needs_empathy"`
	Topic         string `json:"topic"`
	VisualTask    string `json:"visual_task"`
	FaceTextClash bool   `json:"face_text_clash"`
	Reason        string `json:"reason,omitempty"`
}

// HintFromInsight 将分类结果转为融合起点 Hint（不含 QuickDetect）。
func HintFromInsight(ins UtteranceInsight) Hint {
	h := Hint{
		UserMood:     ins.UserMood,
		Intent:       ins.Intent,
		NeedsEmpathy: ins.NeedsEmpathy,
		Topic:        ins.Topic,
		Temperature:  0.85,
	}
	if h.UserMood == "" {
		h.UserMood = "neutral"
	}
	if h.Intent == "" {
		h.Intent = "chat"
	}
	if ins.NeedsEmpathy || ins.Intent == "vent" {
		h.Temperature = 0.75
	}
	if ins.Intent == "joke" {
		h.Temperature = 0.9
	}
	return h
}

// NormalizeVisualTask 规范化 visual_task 字段。
func NormalizeVisualTask(task string) string {
	switch strings.ToLower(strings.TrimSpace(task)) {
	case VisualTaskObject:
		return VisualTaskObject
	case VisualTaskScene:
		return VisualTaskScene
	case VisualTaskFaceConsistency, "face":
		return VisualTaskFaceConsistency
	default:
		return VisualTaskNone
	}
}
