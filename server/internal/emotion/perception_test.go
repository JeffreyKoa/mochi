package emotion

import (
	"testing"

	"github.com/mochi-ai/server/internal/vision"
)

func TestBuildEarlyHint_AcousticSad(t *testing.T) {
	h := BuildEarlyHint(AcousticHint{Mood: "sad", Confidence: 0.9}, vision.EmptyHint(), 0.65, 0.6)
	if !h.NeedsEmpathy || h.UserMood != "sad" {
		t.Fatalf("expected acoustic empathy, got %+v", h)
	}
}

func TestBuildHintFromBuffer_VisualEmpathy(t *testing.T) {
	buf := &PerceptionBuffer{}
	buf.MarkText("我没事")
	buf.MarkAcoustic(EmptyAcousticHint())
	buf.MarkVisual(vision.Hint{
		Focus:                vision.FocusOwnerFace,
		UserExpression:       "sad",
		ExpressionConfidence: 0.85,
		Note:                 "主人看起来难过",
	})
	h := BuildHintFromBuffer(buf, 0.65, 0.6)
	if !h.NeedsEmpathy {
		t.Fatalf("expected visual empathy on neutral text, got %+v", h)
	}
}

func TestShouldEarlyAnimate(t *testing.T) {
	if !ShouldEarlyAnimate(Hint{NeedsEmpathy: true, UserMood: "stressed"}, 0.65) {
		t.Fatal("empathy should early animate")
	}
	if ShouldEarlyAnimate(Hint{UserMood: "neutral", Intent: "chat"}, 0.65) {
		t.Fatal("neutral should not early animate")
	}
}
