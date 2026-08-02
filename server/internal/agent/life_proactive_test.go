package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

func TestRuntime_GenerateLifeNudge(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "有点想你了"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)

	msg, err := rt.GenerateLifeNudge(context.Background(), 1, 1, "love", models.LifeState{
		PetID: 1, Love: 15, Mood: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "想") {
		t.Fatalf("expected brain reply, got %q", msg)
	}
	found := false
	for _, m := range mockAI.lastMessages {
		if strings.Contains(m.Content, "life_nudge") && strings.Contains(m.Content, "love") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected life_nudge context in prompt")
	}
}
