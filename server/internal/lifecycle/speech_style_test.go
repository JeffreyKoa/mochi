package lifecycle

import (
	"strings"
	"testing"
)

func TestDefaultSpeechStyle_byStage(t *testing.T) {
	stages := []string{"newborn", "child", "prime", "elder"}
	for _, stage := range stages {
		got := DefaultSpeechStyle(stage, "cat")
		if got == "" {
			t.Fatalf("stage %s: empty style", stage)
		}
	}
}

func TestDefaultSpeechStyle_tiger(t *testing.T) {
	got := DefaultSpeechStyle("youth", "tiger")
	if !strings.Contains(got, "沉稳") {
		t.Fatalf("expected tiger modifier, got %q", got)
	}
}
