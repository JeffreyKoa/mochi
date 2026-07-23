package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

const proactiveCountPrefix = "mochi:proactive:count:"

type Scheduler struct {
	db      *gorm.DB
	rdb     *redis.Client
	ai      *ai.Provider
	bond    *bond.Service
	cfg     config.CompanionConfig
	broadcaster life.StateBroadcaster
	done    chan struct{}
}

func NewScheduler(db *gorm.DB, rdb *redis.Client, aiProvider *ai.Provider, bondSvc *bond.Service, cfg config.CompanionConfig, hub life.StateBroadcaster) *Scheduler {
	return &Scheduler{
		db:          db,
		rdb:         rdb,
		ai:          aiProvider,
		bond:        bondSvc,
		cfg:         cfg,
		broadcaster: hub,
		done:        make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	if !s.cfg.ProactiveEnabled {
		log.Println("[Companion] proactive disabled")
		return
	}
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.scanAll()
			case <-s.done:
				ticker.Stop()
				return
			}
		}
	}()
	log.Println("[Companion] scheduler started (every 30 min)")
}

func (s *Scheduler) Stop() {
	close(s.done)
}

func (s *Scheduler) scanAll() {
	if s.inQuietHours() {
		return
	}

	var pets []models.Pet
	s.db.Preload("LifeState").Find(&pets)

	for _, pet := range pets {
		s.scanPet(context.Background(), pet)
	}
}

func (s *Scheduler) scanPet(ctx context.Context, pet models.Pet) {
	if !s.canSendToday(ctx, pet.ID) {
		return
	}

	state := models.LifeState{Mood: 70, Love: 60, Hungry: 30, Energy: 80}
	if pet.LifeState != nil {
		state = *pet.LifeState
	}

	bondProfile, _ := s.bond.GetOrCreate(ctx, pet.ID)

	trigger, memorySnippet, animation := s.pickTrigger(ctx, pet, state, bondProfile)
	if trigger == "" {
		return
	}

	msg, err := s.generateMessage(ctx, pet, bondProfile, state, trigger, memorySnippet)
	if err != nil || msg == "" {
		return
	}

	var user models.User
	if s.db.First(&user, pet.UserID).Error != nil {
		return
	}

	if s.broadcaster != nil {
		s.broadcaster.SendProactive(user.ID, msg, animation)
	}
	s.incrementDailyCount(ctx, pet.ID)
	log.Printf("[Companion] proactive sent pet=%d trigger=%s", pet.ID, trigger)
}

type triggerKind string

const (
	triggerEmotionFollowUp triggerKind = "emotion_followup"
	triggerEventFollowUp   triggerKind = "event_followup"
	triggerMissYou         triggerKind = "miss_you"
	triggerMorning         triggerKind = "morning"
	triggerLifeState       triggerKind = "life_state"
)

func (s *Scheduler) pickTrigger(ctx context.Context, pet models.Pet, state models.LifeState, bondProfile models.BondProfile) (triggerKind, string, string) {
	now := time.Now()
	hoursSince := time.Since(state.LastInteraction).Hours()

	if bondProfile.LastMoodAt.After(now.Add(-24*time.Hour)) && emotion.IsNegativeMood(bondProfile.LastMoodTag) {
		if s.cfg.FollowUpEnabled && hoursSince > 2 {
			return triggerEmotionFollowUp, bondProfile.LastMoodTag, "concerned"
		}
	}

	var eventMem models.Memory
	err := s.db.Where("pet_id = ? AND type = ? AND content LIKE ?", pet.ID, "event", "%明天%").
		Order("created_at DESC").First(&eventMem).Error
	if err == nil && eventMem.CreatedAt.Before(now.Add(-12*time.Hour)) {
		return triggerEventFollowUp, eventMem.Content, "happy"
	}

	if state.Hungry > 80 || state.Energy < 20 {
		return triggerLifeState, fmt.Sprintf("hungry=%d energy=%d", state.Hungry, state.Energy), "sad"
	}

	hour := now.Hour()
	if s.cfg.MorningGreeting && hour >= 8 && hour < 9 && hoursSince > 4 {
		return triggerMorning, "", "happy"
	}

	if hoursSince > 24 && bondProfile.RapportLevel > 30 {
		animation := "sad"
		if bondProfile.RapportLevel >= 60 {
			animation = "idle"
		}
		return triggerMissYou, "", animation
	}

	return "", "", ""
}

func (s *Scheduler) generateMessage(ctx context.Context, pet models.Pet, bondProfile models.BondProfile, state models.LifeState, trigger triggerKind, snippet string) (string, error) {
	if s.ai == nil {
		return s.fallbackMessage(trigger, pet.Name, bondProfile), nil
	}

	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)

	prompt := fmt.Sprintf(`你是桌宠 %s，给主人写一条主动消息（50字以内，口语，第一人称，适合语音朗读）。
性格：%s，说话风格：%s
投缘度：%d/100
触发原因：%s
相关记忆：%s
自身状态：心情%d 饥饿%d 精力%d

要求：自然、像伙伴关心主人，不要像通知推送。只输出消息正文。`,
		pet.Name, personality.Traits, personality.SpeechStyle,
		bondProfile.RapportLevel, trigger, orDefault(snippet, "无"),
		state.Mood, state.Hungry, state.Energy,
	)

	resp, err := s.ai.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: "user", Content: prompt}},
		Temperature: 0.85,
		MaxTokens:   100,
	})
	if err != nil {
		return s.fallbackMessage(trigger, pet.Name, bondProfile), nil
	}
	return strings.TrimSpace(resp.Content), nil
}

func (s *Scheduler) fallbackMessage(trigger triggerKind, petName string, bond models.BondProfile) string {
	switch trigger {
	case triggerEmotionFollowUp:
		return "昨天你好像不太开心…今天好点了吗？"
	case triggerEventFollowUp:
		return "之前你说的事怎么样了？我一直记得呢。"
	case triggerMorning:
		return "早啊～今天打算干嘛？"
	case triggerLifeState:
		return "我有点饿了…不过你忙的话先忙就好。"
	default:
		if bond.RapportLevel >= 60 {
			return "好久没聊了，有点想你。"
		}
		return "主人…是不是把我忘了..."
	}
}

func (s *Scheduler) inQuietHours() bool {
	if len(s.cfg.QuietHours) < 2 {
		return false
	}
	hour := time.Now().Hour()
	start, end := s.cfg.QuietHours[0], s.cfg.QuietHours[1]
	if start > end {
		return hour >= start || hour < end
	}
	return hour >= start && hour < end
}

func (s *Scheduler) canSendToday(ctx context.Context, petID uint64) bool {
	max := s.cfg.MaxDailyProactive
	if max <= 0 {
		max = 3
	}
	key := fmt.Sprintf("%s%d:%s", proactiveCountPrefix, petID, time.Now().Format("2006-01-02"))
	count, err := s.rdb.Get(ctx, key).Int()
	if err != nil {
		return true
	}
	return count < max
}

func (s *Scheduler) incrementDailyCount(ctx context.Context, petID uint64) {
	key := fmt.Sprintf("%s%d:%s", proactiveCountPrefix, petID, time.Now().Format("2006-01-02"))
	s.rdb.Incr(ctx, key)
	s.rdb.Expire(ctx, key, 48*time.Hour)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
