package chat

import (
	"testing"
)

func TestNeedsWebSearch(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"今天深圳台风怎么样", true},
		{"帮我查一下苹果股价", true},
		{"把这条新闻梳理总结一下", true},
		{"你好", false},
		{"两加二等于几", false},
		{"明天9点提醒我开会", false},
	}
	for _, c := range cases {
		if got := NeedsWebSearch(c.msg); got != c.want {
			t.Fatalf("%q => %v want %v", c.msg, got, c.want)
		}
	}
}
