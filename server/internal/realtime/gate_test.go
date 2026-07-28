package realtime

import (
	"context"
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func testGateFastpath() config.GateFastpath {
	return config.GateFastpath{}.ApplyDefaults()
}

func TestResponseGate_FastPath(t *testing.T) {
	g := NewResponseGate(
		config.RealtimeGate{Enabled: true, MaxChars: 200, TimeoutMS: 1000, MaxTokens: 20},
		testGateFastpath(),
		"test prompt",
		"key",
		"https://example.com/v1",
	)
	ctx := context.Background()

	tests := []struct {
		text         string
		wantOK       bool
		reasonPrefix string
	}{
		{"", false, "empty"},
		{"好", true, "fastpath:address:"},
		{"今天天气怎么样？", true, "fastpath:question_mark"},
		{"我准备吃午饭了", true, "fastpath:share:"},
		{"刚才觉得好累啊", true, "fastpath:share:"},
	}

	for _, tt := range tests {
		ok, reason := g.Decide(ctx, tt.text, "Mochi")
		if ok != tt.wantOK {
			t.Fatalf("Decide(%q) ok=%v, want %v (reason=%q)", tt.text, ok, tt.wantOK, reason)
		}
		if tt.reasonPrefix != "" && !strings.HasPrefix(reason, tt.reasonPrefix) {
			t.Fatalf("Decide(%q) reason=%q, want prefix %q", tt.text, reason, tt.reasonPrefix)
		}
	}
}
