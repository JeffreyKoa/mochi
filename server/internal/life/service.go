package life

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

type StateBroadcaster interface {
	BroadcastState(userID uint64, state models.LifeState, animation string)
	SendProactive(userID uint64, message, animation string) bool
}

type Service struct {
	db              *gorm.DB
	hub             StateBroadcaster
	done            chan struct{}
	mu              sync.Mutex
	lastTriggerSent map[string]time.Time
}

func NewService(db *gorm.DB, hub StateBroadcaster) *Service {
	return &Service{
		db:              db,
		hub:             hub,
		done:            make(chan struct{}),
		lastTriggerSent: make(map[string]time.Time),
	}
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
		state.Knowledge = clampInt(int(state.Knowledge) + 1)
	case "feed":
		state.Hungry = clampInt(int(state.Hungry) - 30)
		state.Mood = clampInt(int(state.Mood) + 10)
		state.Health = clampInt(int(state.Health) + 5)
		state.Love = clampInt(int(state.Love) + 2)
	case "touch":
		state.Love = clampInt(int(state.Love) + 1)
		state.Mood = clampInt(int(state.Mood) + 3)
	case "play":
		state.Mood = clampInt(int(state.Mood) + 15)
		state.Energy = clampInt(int(state.Energy) - 10)
		state.Curiosity = clampInt(int(state.Curiosity) + 5)
		state.Love = clampInt(int(state.Love) + 5)
	}

	state.LastInteraction = time.Now()
	state.UpdatedAt = time.Now()
	s.db.Save(&state)

	animation := s.animationForState(state, eventType)

	if s.hub != nil {
		var pet models.Pet
		if s.db.First(&pet, petID).Error == nil {
			s.hub.BroadcastState(pet.UserID, state, animation)
		}
	}

	return state, animation, nil
}

func (s *Service) animationForState(state models.LifeState, eventType string) string {
	switch eventType {
	case "touch", "play":
		return "happy"
	case "feed":
		return "eat"
	}
	if state.Sleep > 80 || state.Energy < 20 {
		return "sleep"
	}
	if state.Hungry > 80 || state.Mood < 30 {
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
	log.Println("[LifeEngine] ticker started (every 5 min with 8-state & triggers)")
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
	var pet models.Pet
	hasPet := s.db.First(&pet, state.PetID).Error == nil

	energyDecay := 1
	if hasPet {
		switch pet.LifeStage {
		case "elder", "twilight":
			energyDecay = 2
		case "youth", "prime":
			energyDecay = 0
		}
	}

	state.Hungry = clampInt(int(state.Hungry) + 1)
	state.Energy = clampInt(int(state.Energy) - energyDecay)
	if state.Mood > 0 {
		state.Mood = clampInt(int(state.Mood) - 1)
	}

	now := time.Now()
	hour := now.Hour()
	// 夜间作息规则：困倦度增加
	if hour >= 23 || hour < 7 {
		state.Sleep = clampInt(int(state.Sleep) + 3)
	} else if hour >= 7 && hour < 10 && state.Sleep > 40 {
		state.Sleep = clampInt(int(state.Sleep) - 5)
		state.Energy = clampInt(int(state.Energy) + 5)
	}

	hoursSince := now.Sub(state.LastInteraction).Hours()
	if hoursSince > 6 {
		state.Love = clampInt(int(state.Love) - 1)
		state.Mood = clampInt(int(state.Mood) - 2)
	}

	state.UpdatedAt = now
	s.db.Save(state)

	if hasPet && s.hub != nil {
		s.checkTriggers(pet.UserID, *state)
		animation := s.animationForState(*state, "")
		s.hub.BroadcastState(pet.UserID, *state, animation)
	}
}

const triggerCooldown = 30 * time.Minute

// checkTriggers 检查生命数值临界阈值 (Hungry>=80, Sleep>=80, Love<=20)，带 30 分钟冷却控制
func (s *Service) checkTriggers(userID uint64, state models.LifeState) {
	if s.hub == nil {
		return
	}

	var triggerType, message, animation string
	if state.Hungry >= 80 {
		triggerType = "hungry"
		message = "主人，肚肚咕咕叫了，想吃好吃的..."
		animation = "sad"
	} else if state.Sleep >= 80 {
		triggerType = "sleep"
		message = "眼睛快睁不开了，需要抱抱睡觉 zzz..."
		animation = "sleep"
	} else if state.Love <= 20 {
		triggerType = "love"
		message = "主人...好久没理我了，是不是不爱我了..."
		animation = "sad"
	} else {
		return
	}

	key := fmt.Sprintf("%d:%s", userID, triggerType)
	s.mu.Lock()
	if last, ok := s.lastTriggerSent[key]; ok && time.Since(last) < triggerCooldown {
		s.mu.Unlock()
		return
	}
	s.lastTriggerSent[key] = time.Now()
	s.mu.Unlock()

	s.hub.SendProactive(userID, message, animation)
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
