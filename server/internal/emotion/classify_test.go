package emotion

import (
	"testing"

	"github.com/mochi-ai/server/internal/vision"
)

func TestHintFromInsight_Vent(t *testing.T) {
	h := HintFromInsight(UtteranceInsight{
		UserMood:     "stressed",
		Intent:       "vent",
		NeedsEmpathy: true,
	})
	if h.UserMood != "stressed" || h.Intent != "vent" || !h.NeedsEmpathy {
		t.Fatalf("unexpected hint: %+v", h)
	}
	if h.Temperature != 0.75 {
		t.Fatalf("expected lower temperature, got %v", h.Temperature)
	}
}

func TestFallbackInsight_AcousticSad(t *testing.T) {
	ins := FallbackInsight(AcousticHint{Mood: "sad", Confidence: 0.9}, vision.EmptyHint())
	if ins.Intent != "vent" || !ins.NeedsEmpathy || ins.UserMood != "sad" {
		t.Fatalf("expected acoustic vent, got %+v", ins)
	}
}

func TestBuildFinalPerception_FaceClash(t *testing.T) {
	face := vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "tired",
		ExpressionConfidence: 0.85,
		Note:                 "疲惫",
	}
	ins := UtteranceInsight{
		UserMood:      "neutral",
		Intent:        "chat",
		FaceTextClash: true,
		VisualTask:    VisualTaskFaceConsistency,
	}
	state := BuildFinalPerception("我没事", EmptyAcousticHint(), face, ins, 0.65, 0.6)
	if !state.Hint.NeedsEmpathy {
		t.Fatalf("expected empathy on clash, got %+v", state.Hint)
	}
	if state.Hint.Intent != "vent" {
		t.Fatalf("expected vent intent, got %s", state.Hint.Intent)
	}
}

func TestNormalizeVisualTask(t *testing.T) {
	if NormalizeVisualTask("OBJECT") != VisualTaskObject {
		t.Fatal("object normalize failed")
	}
}
