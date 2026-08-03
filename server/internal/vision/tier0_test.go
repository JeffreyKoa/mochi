package vision

import "testing"

func TestTier0PauseHint_Unfinished(t *testing.T) {
	expr, composing := Tier0PauseHint("因为", &FaceProbeIn{Detected: true, Score: 0.8})
	if !composing || expr != "thinking" {
		t.Fatalf("expected thinking composing, got %s composing=%v", expr, composing)
	}
}

func TestTier0PauseHint_WeatherPartial(t *testing.T) {
	_, composing := Tier0PauseHint("深圳天气", nil)
	if !composing {
		t.Fatal("short partial without punct should be composing")
	}
}
