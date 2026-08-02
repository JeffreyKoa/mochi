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

// ProactiveBrain 生命临界主动关怀：台词由 agent Runtime 生成，life 层只负责阈值检测。
type ProactiveBrain interface {
	GenerateLifeNudge(ctx context.Context, userID, petID uint64, triggerType string, state models.LifeState) (string, error)
}

type Service struct {
	db              *gorm.DB
	hub             StateBroadcaster
	brain           ProactiveBrain
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

// SetProactiveBrain 注入大脑（agent.Runtime 实现）；未注入时不发主动台词。
func (s *Service) SetProactiveBrain(b ProactiveBrain) {
	s.brain = b
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

const neglectThreshold = 7 * 24 * time.Hour

// IsNeglected reports whether the pet has had no interaction for 7+ days.
func IsNeglected(lastInteraction time.Time) bool {
	return !lastInteraction.IsZero() && time.Since(lastInteraction) > neglectThreshold
}

// ApplyNeglectIfNeeded persists lonely/sad FSM when the owner has been away 7+ days.
func (s *Service) ApplyNeglectIfNeeded(ctx context.Context, petID uint64) bool {
	if s == nil || s.db == nil {
		return false
	}
	state, err := s.GetState(ctx, petID)
	if err != nil {
		return false
	}
	if !IsNeglected(state.LastInteraction) {
		return false
	}
	if state.EmotionState == "sad" && state.Mood <= 20 {
		return false
	}
	state.EmotionState = "sad"
	state.Mood = 20
	state.UpdatedAt = time.Now()
	if err := s.db.WithContext(ctx).Save(&state).Error; err != nil {
		return false
	}
	s.BroadcastStateDirect(petID, state, "sad")
	return true
}

func (s *Service) BroadcastStateDirect(petID uint64, state models.LifeState, animation string) {
	if s.hub != nil {
		var pet models.Pet
		if s.db.First(&pet, petID).Error == nil {
			s.hub.BroadcastState(pet.UserID, state, animation)
		}
	}
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

// checkTriggers 检查生命数值临界阈值 (Hungry>=80, Sleep>=80, Love<=20)，带 30 分钟冷却；台词由大脑生成。
func (s *Service) checkTriggers(userID uint64, state models.LifeState) {
	if s.hub == nil || s.brain == nil {
		return
	}

	if s.db != nil {
		var user models.User
		if s.db.First(&user, userID).Error != nil || !user.ProactiveEnabled {
			return
		}
	}

	var triggerType string
	if state.Hungry >= 80 {
		triggerType = "hungry"
	} else if state.Sleep >= 80 {
		triggerType = "sleep"
	} else if state.Love <= 20 {
		triggerType = "love"
	} else {
		return
	}

	key := fmt.Sprintf("%d:%s", userID, triggerType)
	s.mu.Lock()
	if last, ok := s.lastTriggerSent[key]; ok && time.Since(last) < triggerCooldown {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	petID := state.PetID
	animation := s.animationForState(state, triggerType)

	// 异步调用大脑，避免阻塞 5 分钟 tick
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		msg, err := s.brain.GenerateLifeNudge(ctx, userID, petID, triggerType, state)
		if err != nil || msg == "" {
			if err != nil {
				log.Printf("[LifeEngine] life nudge brain failed user=%d trigger=%s: %v", userID, triggerType, err)
			}
			return
		}

		s.mu.Lock()
		if last, ok := s.lastTriggerSent[key]; ok && time.Since(last) < triggerCooldown {
			s.mu.Unlock()
			return
		}
		s.lastTriggerSent[key] = time.Now()
		s.mu.Unlock()

		if s.hub.SendProactive(userID, msg, animation) {
			log.Printf("[LifeEngine] proactive sent user=%d trigger=%s", userID, triggerType)
		}
	}()
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
