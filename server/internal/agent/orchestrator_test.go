package agent

import (
	"context"
	"testing"
	"time"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/memory"
)

func TestOrchestrator_PrepareChatContext(t *testing.T) {
	// Initialize in-memory mock/fake dependencies where possible or nil-safe services
	memSvc := memory.NewService(nil, nil, nil, nil)
	emoSvc := emotion.NewService(nil, nil)
	briefSvc := brief.NewService(nil, config.GrowthConfig{})
	bondSvc := bond.NewService(nil)

	orch := NewOrchestrator(memSvc, emoSvc, briefSvc, bondSvc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	agentCtx := orch.PrepareChatContext(ctx, 1, "今天天气真好，我很开心！")

	if agentCtx.EmotionHint.UserMood == "" {
		t.Errorf("expected non-empty emotion hint user mood")
	}
}
