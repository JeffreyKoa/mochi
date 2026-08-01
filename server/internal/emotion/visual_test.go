package emotion

import (
	"testing"

	"github.com/mochi-ai/server/internal/vision"
)

func TestMergeVisualHint_ConfidenceGate(t *testing.T) {
	h := Hint{UserMood: "neutral", Intent: "chat"}
	v := vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "tired",
		ExpressionConfidence: 0.5,
		Note:                 "看起来很累",
	}
	out := MergeVisualHint(h, v, 0.6)
	if out.VisualNote != "" {
		t.Fatal("should skip low confidence")
	}
	v.ExpressionConfidence = 0.8
	out = MergeVisualHint(h, v, 0.6)
	if out.VisualNote == "" || out.VisualFocus != "owner_face" {
		t.Fatal("should merge high confidence owner_face")
	}
}

func TestMergeVisualHint_ObjectDoesNotChangeMood(t *testing.T) {
	h := Hint{UserMood: "happy", Intent: "joke", NeedsEmpathy: false}
	v := vision.Hint{
		Focus:         vision.FocusObject,
		ObjectSummary: "一个红色马克杯",
		Note:          "主人举着一个红色马克杯",
	}
	out := MergeVisualHint(h, v, 0.6)
	if out.UserMood != "happy" || out.Intent != "joke" {
		t.Fatalf("mood/intent should not change: mood=%s intent=%s", out.UserMood, out.Intent)
	}
	if out.VisualNote == "" || out.VisualFocus != "object" {
		t.Fatalf("expected object visual note, got note=%q focus=%q", out.VisualNote, out.VisualFocus)
	}
}

func TestMergeVisualHint_SceneDoesNotChangeMood(t *testing.T) {
	h := Hint{UserMood: "neutral", Intent: "chat"}
	v := vision.Hint{
		Focus:        vision.FocusScene,
		SceneSummary: "窗外阴天，室内灯光明亮",
		Note:         "外面看起来阴阴的",
	}
	out := MergeVisualHint(h, v, 0.6)
	if out.UserMood != "neutral" {
		t.Fatalf("mood should not change: %s", out.UserMood)
	}
	if out.VisualFocus != "scene" {
		t.Fatalf("expected scene focus, got %q", out.VisualFocus)
	}
}
