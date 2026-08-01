package vision

// Focus 视觉注意力焦点。
type Focus string

const (
	FocusSkip      Focus = "skip"
	FocusOwnerFace Focus = "owner_face"
	FocusObject    Focus = "object" // V1.5
	FocusScene     Focus = "scene"  // V2
)

// Hint 视觉理解结果（结构化，供融合与 Prompt 注入）。
type Hint struct {
	Focus                Focus   `json:"focus"`
	UserExpression       string  `json:"user_expression"` // neutral|tired|sad|happy|anxious|unknown
	ExpressionConfidence float64 `json:"expression_confidence"`
	ObjectSummary        string  `json:"object_summary,omitempty"`
	SceneSummary         string  `json:"scene_summary,omitempty"`
	Note                 string  `json:"note,omitempty"` // 供 Prompt 的一句话
	Skipped              bool    `json:"skipped,omitempty"`
	SkipReason           string  `json:"skip_reason,omitempty"`
}

// EmptyHint 表示未启用、无帧或识别跳过。
func EmptyHint() Hint {
	return Hint{Focus: FocusSkip, UserExpression: "unknown", Skipped: true}
}

// IsUsable 是否有可用的视觉信号（表情或摘要）。
func (h Hint) IsUsable() bool {
	if h.Skipped || h.Focus == FocusSkip {
		return false
	}
	switch h.Focus {
	case FocusOwnerFace:
		return h.Note != "" || h.ExpressionConfidence > 0
	case FocusObject:
		return h.Note != "" || h.ObjectSummary != ""
	case FocusScene:
		return h.Note != "" || h.SceneSummary != ""
	default:
		return h.Note != ""
	}
}
