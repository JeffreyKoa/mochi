package tools

import (
	"strings"

	"github.com/mochi-ai/server/internal/emotion"
)

var toolActionKeywords = []string{
	"提醒", "记得", "别忘了", "记一下", "记下来", "记下", "通知我", "通知一下",
	"待办", "帮我记", "设个提醒", "设提醒", "闹钟",
}

// NeedsToolAction reports whether the user message likely requires reminder/todo tools.
func NeedsToolAction(userMsg string, hint emotion.Hint) bool {
	if hint.Intent == "plan" {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(userMsg))
	for _, kw := range toolActionKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func looksLikeReminder(msg string) bool {
	msg = strings.TrimSpace(msg)
	if strings.Contains(msg, "待办") {
		return false
	}
	for _, kw := range []string{"提醒", "记得", "别忘了", "通知", "开会", "会议", "闹钟"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	if _, ok := ParseScheduledTime(msg); ok {
		return strings.Contains(msg, "点") || strings.Contains(msg, "时") || strings.Contains(msg, "半")
	}
	return false
}

func looksLikeTodo(msg string) bool {
	msg = strings.TrimSpace(msg)
	for _, kw := range []string{"记一下", "记下来", "记下", "帮我记", "待办"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
