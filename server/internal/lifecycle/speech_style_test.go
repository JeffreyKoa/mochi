package lifecycle

import (
	"strings"
	"testing"
)

func TestDefaultSpeechStyle_byStage(t *testing.T) {
	tests := []struct {
		stage string
		want  string
	}{
		{"newborn", "像普通年轻人"},
		{"child", "像小孩一样口语化"},
		{"prime", "像三十岁左右"},
		{"elder", "像长辈"},
	}
	for _, tt := range tests {
		got := DefaultSpeechStyle(tt.stage, "cat")
		if got == "" {
			t.Fatalf("stage %s: empty style", tt.stage)
		}
		if tt.want != "" && !strings.Contains(got, tt.want) {
			t.Fatalf("stage %s: got %q, want substring %q", tt.stage, got, tt.want)
		}
	}
}

func TestDefaultSpeechStyle_tiger(t *testing.T) {
	got := DefaultSpeechStyle("youth", "tiger")
	if !strings.Contains(got, "沉稳") {
		t.Fatalf("expected tiger modifier, got %q", got)
	}
}
