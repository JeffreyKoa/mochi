package emotion

import (
	"testing"

	"github.com/mochi-ai/server/internal/vision"
)

func TestApplyVisualEmpathy_neutralFaceSad(t *testing.T) {
	h := Hint{UserMood: "neutral", Intent: "chat"}
	v := vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "sad",
		ExpressionConfidence: 0.85,
		Note:                 "主人眼眶泛红",
	}
	out := applyVisualEmpathy(h, v, 0.6)
	if !out.NeedsEmpathy || out.UserMood != "sad" {
		t.Fatalf("expected visual empathy, got mood=%s empathy=%v", out.UserMood, out.NeedsEmpathy)
	}
}

func TestApplyVisualEmpathy_forcedSmile(t *testing.T) {
	h := Hint{UserMood: "happy", Intent: "joke"}
	v := vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "sad",
		ExpressionConfidence: 0.8,
		Note:                 "在笑但眼里有泪",
	}
	out := applyVisualEmpathy(h, v, 0.6)
	if !out.NeedsEmpathy {
		t.Fatal("expected forced smile empathy")
	}
}

func TestApplyVisualEmpathy_acousticStack(t *testing.T) {
	h := Hint{UserMood: "sad", Intent: "vent", NeedsEmpathy: true}
	v := vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "sad",
		ExpressionConfidence: 0.9,
		Note:                 "视觉也看到难过",
	}
	out := applyVisualEmpathy(h, v, 0.6)
	if out.UserMood != "sad" || out.VisualNote == "" {
		t.Fatalf("should stack note on acoustic, mood=%s note=%q", out.UserMood, out.VisualNote)
	}
}
