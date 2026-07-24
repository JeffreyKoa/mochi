package text

import "testing"

func TestStripActionParentheticals(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"（把整颗小脑袋都埋进你掌心）嗯嗯~", "嗯嗯~"},
		{"(微笑)你好呀", "你好呀"},
		{"*歪头*怎么啦？", "怎么啦？"},
		{"先（蹭了蹭）再（点点头）好的", "先再好的"},
		{"正常对话没有动作", "正常对话没有动作"},
	}
	for _, tt := range tests {
		got := StripActionParentheticals(tt.in)
		if got != tt.want {
			t.Errorf("StripActionParentheticals(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStreamSanitizer(t *testing.T) {
	var ss StreamSanitizer
	got := ss.Feed("（把")
	if got != "" {
		t.Fatalf("partial open paren should emit nothing, got %q", got)
	}
	got = ss.Feed("头埋进掌心）嗯嗯")
	if got != "嗯嗯" {
		t.Fatalf("after close paren, got %q", got)
	}
	if tail := ss.Flush(); tail != "" {
		t.Fatalf("flush should be empty, got %q", tail)
	}
}
