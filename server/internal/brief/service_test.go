package brief

import (
	"testing"

	"github.com/mochi-ai/server/internal/models"
)

func TestCompileEntriesRespectsBudget(t *testing.T) {
	entries := make([]models.UserBriefEntry, 30)
	for i := range entries {
		entries[i] = models.UserBriefEntry{
			Category:   "preference",
			Content:    "测试偏好条目内容比较长用来占字符",
			Importance: float32(30 - i),
		}
	}
	text := CompileEntries(entries, 1400)
	if len([]rune(text)) > 1400 {
		t.Fatalf("compiled text exceeds budget: %d runes", len([]rune(text)))
	}
	if text == "" {
		t.Fatal("expected non-empty compiled text")
	}
}

func TestCompileEntriesPrefersHighImportance(t *testing.T) {
	entries := []models.UserBriefEntry{
		{Category: "preference", Content: "低优先级", Importance: 0.1},
		{Category: "preference", Content: "高优先级", Importance: 0.99},
	}
	text := CompileEntries(entries, 1400)
	if text == "" {
		t.Fatal("expected compiled text")
	}
	if !contains(text, "高优先级") {
		t.Fatalf("high importance entry missing: %s", text)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
