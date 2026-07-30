package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

var cnLoc = time.FixedZone("CST", 8*3600)

const daysPerYear = 365

type AgeInfo struct {
	AgeDays       int
	AgeYears      int
	AgeDaysInYear int
	MaxDays       int
	RemainingDays int
	Ratio         float64
	Stage         string
	IsAlive       bool
}

type StageBroadcaster interface {
	SendLifeStageChanged(userID uint64, data map[string]interface{})
}

type NeglectApplier interface {
	ApplyNeglectIfNeeded(ctx context.Context, petID uint64) bool
}

type Service struct {
	db   *gorm.DB
	hub  StageBroadcaster
	life NeglectApplier
	done chan struct{}
}

func NewService(db *gorm.DB, hub StageBroadcaster, life NeglectApplier) *Service {
	return &Service{db: db, hub: hub, life: life, done: make(chan struct{})}
}

func DefaultMaxAgeYears(species string) float32 {
	switch species {
	case "dog_small":
		return 15
	case "dog_medium":
		return 13
	case "dog_large":
		return 11
	case "tiger":
		return 20
	case "lion":
		return 18
	default:
		return 18
	}
}

func CalendarAgeDays(bornAt, now time.Time) int {
	if bornAt.IsZero() {
		return 0
	}
	born := bornAt.In(cnLoc)
	cur := now.In(cnLoc)
	bornDate := time.Date(born.Year(), born.Month(), born.Day(), 0, 0, 0, 0, cnLoc)
	curDate := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cnLoc)
	days := int(curDate.Sub(bornDate).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func StageFromRatio(ratio float64, remainingDays int) string {
	if remainingDays <= 0 || ratio >= 0.98 {
		return "departed"
	}
	switch {
	case ratio < 0.01:
		return "newborn"
	case ratio < 0.05:
		return "juvenile"
	case ratio < 0.15:
		return "child"
	case ratio < 0.35:
		return "youth"
	case ratio < 0.60:
		return "prime"
	case ratio < 0.90:
		return "elder"
	default:
		return "twilight"
	}
}

func ComputeAgeInfo(pet models.Pet, now time.Time) AgeInfo {
	if now.IsZero() {
		now = time.Now()
	}
	maxYears := pet.MaxAgeYears
	if maxYears <= 0 {
		maxYears = DefaultMaxAgeYears(pet.Species)
	}
	maxDays := int(maxYears * daysPerYear)
	if maxDays <= 0 {
		maxDays = 18 * daysPerYear
	}

	bornAt := pet.BornAt
	if bornAt.IsZero() {
		bornAt = pet.CreatedAt
	}
	ageDays := CalendarAgeDays(bornAt, now)
	remaining := maxDays - ageDays
	ratio := 0.0
	if maxDays > 0 {
		ratio = float64(ageDays) / float64(maxDays)
	}

	stage := StageFromRatio(ratio, remaining)
	isAlive := pet.IsAlive && stage != "departed"
	if !pet.IsAlive {
		isAlive = false
		stage = "departed"
	}

	return AgeInfo{
		AgeDays:       ageDays,
		AgeYears:      ageDays / daysPerYear,
		AgeDaysInYear: ageDays % daysPerYear,
		MaxDays:       maxDays,
		RemainingDays: remaining,
		Ratio:         ratio,
		Stage:         stage,
		IsAlive:       isAlive,
	}
}

// NormalizeGender returns male or female; empty defaults to female.
func NormalizeGender(g string) string {
	if g == "male" {
		return "male"
	}
	return "female"
}

func (s *Service) SyncPet(ctx context.Context, pet *models.Pet) (AgeInfo, bool, error) {
	info := ComputeAgeInfo(*pet, time.Now())

	if s.life != nil {
		s.life.ApplyNeglectIfNeeded(ctx, pet.ID)
	}

	changed := info.Stage != pet.LifeStage || info.IsAlive != pet.IsAlive

	if !changed {
		return info, false, nil
	}

	oldStage := pet.LifeStage
	updates := map[string]interface{}{
		"life_stage": info.Stage,
		"is_alive":   info.IsAlive,
	}
	if err := s.db.WithContext(ctx).Model(pet).Updates(updates).Error; err != nil {
		return info, false, err
	}
	pet.LifeStage = info.Stage
	pet.IsAlive = info.IsAlive

	if oldStage != info.Stage && info.Stage != "departed" {
		s.syncSpeechStyleForStage(ctx, pet, info.Stage)
	}

	if s.hub != nil && oldStage != info.Stage {
		s.hub.SendLifeStageChanged(pet.UserID, map[string]interface{}{
			"life_stage":       info.Stage,
			"life_stage_label": StageLabel(info.Stage),
			"age_days":         info.AgeDays,
			"age_years":        info.AgeYears,
			"age_days_in_year": info.AgeDaysInYear,
			"remaining_days":   info.RemainingDays,
			"is_alive":         info.IsAlive,
		})
	}
	return info, true, nil
}

func (s *Service) syncSpeechStyleForStage(ctx context.Context, pet *models.Pet, stage string) {
	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)
	personality.SpeechStyle = DefaultSpeechStyle(stage, pet.Species)
	personality.StyleNotes = nil
	data, err := json.Marshal(personality)
	if err != nil {
		return
	}
	if err := s.db.WithContext(ctx).Model(pet).Update("personality_json", data).Error; err != nil {
		log.Printf("[Lifecycle] sync speech style pet=%d: %v", pet.ID, err)
		return
	}
	pet.PersonalityJSON = data
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
	log.Println("[Lifecycle] ticker started (every 5 min)")
}

func (s *Service) Stop() {
	close(s.done)
}

func (s *Service) tickAll() {
	var pets []models.Pet
	if err := s.db.Find(&pets).Error; err != nil {
		return
	}
	for i := range pets {
		if _, _, err := s.SyncPet(context.Background(), &pets[i]); err != nil {
			log.Printf("[Lifecycle] sync pet=%d: %v", pets[i].ID, err)
		}
	}
}

func FormatAgeDisplay(info AgeInfo) string {
	return fmt.Sprintf("%d岁%d天 · 还可陪伴 %d 天", info.AgeYears, info.AgeDaysInYear, info.RemainingDays)
}
