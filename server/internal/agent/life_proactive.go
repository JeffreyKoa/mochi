package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/mochi-ai/server/internal/models"
)

// GenerateLifeNudge 生命引擎临界触发：由大脑生成主动关怀台词（禁止在 life 层硬编码）。
func (r *Runtime) GenerateLifeNudge(ctx context.Context, userID, petID uint64, triggerType string, state models.LifeState) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runtime nil")
	}
	instruction := fmt.Sprintf(
		"[SYSTEM_TRIGGER: life_nudge] 生命状态临界: type=%s hungry=%d sleep=%d love=%d mood=%d energy=%d",
		triggerType, state.Hungry, state.Sleep, state.Love, state.Mood, state.Energy,
	)
	out, err := r.Turn(ctx, TurnInput{
		UserID:      userID,
		PetID:       petID,
		Message:     instruction,
		TriggerType: "system_proactive",
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range out.ReplyStream {
		if chunk.Content != "" {
			b.WriteString(chunk.Content)
		}
	}
	return strings.TrimSpace(b.String()), nil
}
