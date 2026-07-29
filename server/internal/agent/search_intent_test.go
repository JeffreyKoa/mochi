package agent

import "testing"

func TestNeedsWebSearch(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"帮我查一下今天的新闻", true},
		{"你好呀", false},
		{"What's the weather today", true},
	}
	for _, c := range cases {
		if got := NeedsWebSearch(c.msg); got != c.want {
			t.Fatalf("NeedsWebSearch(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}
