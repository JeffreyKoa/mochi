package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/vision"
)

// SetTurnPCM 不应清除 speech_start prefetch 已完成的 visual hint。
func TestSetTurnPCM_PreservesVisualPrefetch(t *testing.T) {
	s := NewSession("u1", 1, nil)
	hint := vision.Hint{Focus: vision.FocusOwnerFace, ExpressionConfidence: 0.8}
	s.SetVisualHint(hint)

	s.SetTurnPCM([]byte{1, 2, 3})

	if !s.visualDone() {
		t.Fatal("visualReady should remain true after SetTurnPCM")
	}
	got := s.VisualHint()
	if got.Focus != vision.FocusOwnerFace {
		t.Fatalf("visual hint focus=%q, want owner_face", got.Focus)
	}
}
