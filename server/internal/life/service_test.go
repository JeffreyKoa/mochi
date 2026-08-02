package life

import (
	"context"
	"testing"
	"time"

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

type mockBrain struct {
	msg string
	ch  chan struct{}
}

func (b *mockBrain) GenerateLifeNudge(ctx context.Context, userID, petID uint64, triggerType string, state models.LifeState) (string, error) {
	if b.ch != nil {
		close(b.ch)
	}
	return b.msg, nil
}

func TestLifeService_CheckTriggersAndCooldown(t *testing.T) {
	hub := &mockHub{}
	done := make(chan struct{})
	svc := NewService(nil, hub)
	svc.SetProactiveBrain(&mockBrain{msg: "brain-generated", ch: done})

	state := models.LifeState{
		PetID:  1,
		Hungry: 85,
		Sleep:  20,
		Love:   50,
	}

	svc.checkTriggers(100, state)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("brain not invoked")
	}
	// 异步发送，稍等 hub 收到
	time.Sleep(50 * time.Millisecond)
	if len(hub.proactiveMsgs) != 1 {
		t.Fatalf("expected 1 proactive message, got %d", len(hub.proactiveMsgs))
	}
	if hub.proactiveMsgs[0] != "brain-generated" {
		t.Fatalf("expected brain message, got %q", hub.proactiveMsgs[0])
	}

	svc.checkTriggers(100, state)
	time.Sleep(50 * time.Millisecond)
	if len(hub.proactiveMsgs) != 1 {
		t.Errorf("expected cooldown to prevent duplicate message, got count %d", len(hub.proactiveMsgs))
	}
}

func TestLifeService_NoBrainNoProactive(t *testing.T) {
	hub := &mockHub{}
	svc := NewService(nil, hub)
	state := models.LifeState{PetID: 1, Hungry: 90}
	svc.checkTriggers(100, state)
	time.Sleep(50 * time.Millisecond)
	if len(hub.proactiveMsgs) != 0 {
		t.Fatalf("expected no message without brain, got %d", len(hub.proactiveMsgs))
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

func TestIsNeglected(t *testing.T) {
	if IsNeglected(time.Time{}) {
		t.Fatal("zero time should not be neglected")
	}
	if IsNeglected(time.Now().Add(-48 * time.Hour)) {
		t.Fatal("2 days should not be neglected")
	}
	if !IsNeglected(time.Now().Add(-8 * 24 * time.Hour)) {
		t.Fatal("8 days should be neglected")
	}
}
