package life

import (
	"testing"

	"github.com/mochi-ai/server/internal/models"
)

type mockHub struct {
	proactiveMsgs []string
}

func (m *mockHub) BroadcastState(userID uint64, state models.LifeState, animation string) {}
func (m *mockHub) SendProactive(userID uint64, message, animation string) bool {
	m.proactiveMsgs = append(m.proactiveMsgs, message)
	return true
}

func TestLifeService_CheckTriggersAndCooldown(t *testing.T) {
	hub := &mockHub{}
	svc := NewService(nil, hub)

	state := models.LifeState{
		PetID:  1,
		Hungry: 85,
		Sleep:  20,
		Love:   50,
	}

	// 1st check -> should trigger proactive message
	svc.checkTriggers(100, state)
	if len(hub.proactiveMsgs) != 1 {
		t.Fatalf("expected 1 proactive message, got %d", len(hub.proactiveMsgs))
	}

	// 2nd check immediately -> should be blocked by 30-min cooldown
	svc.checkTriggers(100, state)
	if len(hub.proactiveMsgs) != 1 {
		t.Errorf("expected cooldown to prevent duplicate message, got count %d", len(hub.proactiveMsgs))
	}
}

func TestLifeService_ClampInt(t *testing.T) {
	if clampInt(-10) != 0 {
		t.Errorf("expected clampInt(-10) == 0")
	}
	if clampInt(150) != 100 {
		t.Errorf("expected clampInt(150) == 100")
	}
	if clampInt(42) != 42 {
		t.Errorf("expected clampInt(42) == 42")
	}
}
