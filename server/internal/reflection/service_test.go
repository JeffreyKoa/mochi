package reflection

import "testing"

func TestIsActionableStyleFeedback(t *testing.T) {
	tests := []struct {
		note string
		want bool
	}{
		{"诗意隐喻、星尘晨光意象", false},
		{"用户嫌回复太长", true},
		{"不要小作文，要口语短句", true},
		{"偏好通感式表达", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isActionableStyleFeedback(tt.note); got != tt.want {
			t.Fatalf("note %q: got %v want %v", tt.note, got, tt.want)
		}
	}
}
