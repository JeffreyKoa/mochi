package lifecycle

import (
	"context"
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

type Service struct {
	db   *gorm.DB
	hub  StageBroadcaster
	done chan struct{}
}

func NewService(db *gorm.DB, hub StageBroadcaster) *Service {
	return &Service{db: db, hub: hub, done: make(chan struct{})}
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

func StageLabel(stage string) string {
	switch stage {
	case "newborn":
		return "刚出生"
	case "juvenile":
		return "幼年"
	case "child":
		return "童年"
	case "youth":
		return "青年"
	case "prime":
		return "壮年"
	case "elder":
		return "老年"
	case "twilight":
		return "暮年"
	case "departed":
		return "已告别"
	default:
		return stage
	}
}

func PromptFragment(stage, species string) string {
	base := ""
	switch stage {
	case "newborn":
		base = "你刚出生不久，懵懂黏人，话少，句子要很短很简单。"
	case "juvenile":
		base = "你在幼年期，好奇多动，学习记忆快，可以多问主人问题。"
	case "child":
		base = "你在童年期，活泼爱撒娇，互动热情高。"
	case "youth":
		base = "你在青年期，精力旺盛、话多积极，最能帮主人想事情。"
	case "prime":
		base = "你在壮年期，稳重靠谱，仍保持活泼，是全能陪伴期。"
	case "elder":
		base = "你在老年期，温和爱回忆，更懂主人，但精力下降，句子可稍长、更有阅历。"
	case "twilight":
		base = "你在暮年期，陪伴时间不多了，语气温柔、走心，少开玩笑。"
	case "departed":
		base = "你已经告别，不再说话。"
	default:
		base = "自然陪伴主人。"
	}
	if species == "tiger" || species == "lion" {
		base += " 你是幻想伙伴，气质霸气偏守护，少撒娇。"
	}
	return base
}

func (s *Service) SyncPet(ctx context.Context, pet *models.Pet) (AgeInfo, bool, error) {
	info := ComputeAgeInfo(*pet, time.Now())
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
