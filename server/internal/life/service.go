package life

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

type StateBroadcaster interface {
	BroadcastState(userID uint64, state models.LifeState, animation string)
	SendProactive(userID uint64, message, animation string)
}

type Service struct {
	db   *gorm.DB
	hub  StateBroadcaster
	done chan struct{}
}

func NewService(db *gorm.DB, hub StateBroadcaster) *Service {
	return &Service{db: db, hub: hub, done: make(chan struct{})}
}

func (s *Service) GetState(ctx context.Context, petID uint64) (models.LifeState, error) {
	var state models.LifeState
	err := s.db.First(&state, "pet_id = ?", petID).Error
	return state, err
}

func (s *Service) Interact(ctx context.Context, petID uint64, eventType string) (models.LifeState, string, error) {
	state, err := s.GetState(ctx, petID)
	if err != nil {
		return state, "idle", err
	}

	switch eventType {
	case "chat":
		state.Love = clampInt(int(state.Love) + 3)
		state.Mood = clampInt(int(state.Mood) + 5)
		state.Energy = clampInt(int(state.Energy) - 2)
		state.Hungry = clampInt(int(state.Hungry) + 1)
	case "feed":
		state.Hungry = clampInt(int(state.Hungry) - 30)
		state.Mood = clampInt(int(state.Mood) + 10)
		state.Love = clampInt(int(state.Love) + 2)
	case "touch":
		state.Love = clampInt(int(state.Love) + 1)
		state.Mood = clampInt(int(state.Mood) + 3)
	case "play":
		state.Mood = clampInt(int(state.Mood) + 15)
		state.Energy = clampInt(int(state.Energy) - 10)
		state.Love = clampInt(int(state.Love) + 5)
	}

	state.LastInteraction = time.Now()
	state.UpdatedAt = time.Now()
	s.db.Save(&state)

	animation := s.animationForState(state, eventType)
	return state, animation, nil
}

func (s *Service) animationForState(state models.LifeState, eventType string) string {
	switch eventType {
	case "touch", "play":
		return "happy"
	case "feed":
		return "eat"
	}
	if state.Energy < 20 {
		return "sleep"
	}
	if state.Mood < 30 {
		return "sad"
	}
	return "idle"
}

func (s *Service) StartTicker() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.tickAll()
			case <-s.done:
				ticker.Stop()
				return
			}
		}
	}()
	log.Println("[LifeEngine] ticker started (every 5 min)")
}

func (s *Service) Stop() {
	close(s.done)
}

func (s *Service) tickAll() {
	var states []models.LifeState
	s.db.Find(&states)

	for _, state := range states {
		s.tick(&state)
	}
}

func (s *Service) tick(state *models.LifeState) {
	state.Hungry = clampInt(int(state.Hungry) + 1)
	state.Energy = clampInt(int(state.Energy) - 1)
	if state.Mood > 0 {
		state.Mood = clampInt(int(state.Mood) - 1)
	}

	hoursSince := time.Since(state.LastInteraction).Hours()
	if hoursSince > 6 {
		state.Love = clampInt(int(state.Love) - 1)
		state.Mood = clampInt(int(state.Mood) - 2)
	}
	// Proactive messages handled by companion scheduler

	state.UpdatedAt = time.Now()
	s.db.Save(state)

	if s.hub != nil {
		var pet models.Pet
		if s.db.First(&pet, state.PetID).Error == nil {
			animation := s.animationForState(*state, "")
			s.hub.BroadcastState(pet.UserID, *state, animation)
		}
	}
}

func clampInt(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return uint8(v)
}
