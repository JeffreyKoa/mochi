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
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/tools"
	"github.com/mochi-ai/server/pkg/ai"
)

const proactiveCountPrefix = "mochi:proactive:count:"

type Scheduler struct {
	db          *gorm.DB
	rdb         *redis.Client
	ai          ai.AIProvider
	bond        *bond.Service
	cfg         config.CompanionConfig
	toolsSvc    *tools.Service
	toolsCfg    config.ToolsConfig
	broadcaster life.StateBroadcaster
	rtReminders realtimeReminderDeliverer
	done        chan struct{}
}

type realtimeReminderDeliverer interface {
	SendProactiveReminder(userID, reminderID uint64, message, animation string) bool
}

func NewScheduler(db *gorm.DB, rdb *redis.Client, aiProvider ai.AIProvider, bondSvc *bond.Service, cfg config.CompanionConfig, broadcaster life.StateBroadcaster, toolsSvc *tools.Service, toolsCfg config.ToolsConfig, rtReminders realtimeReminderDeliverer) *Scheduler {
	return &Scheduler{
		db:          db,
		rdb:         rdb,
		ai:          aiProvider,
		bond:        bondSvc,
		cfg:         cfg,
		toolsSvc:    toolsSvc,
		toolsCfg:    toolsCfg,
		broadcaster: broadcaster,
		rtReminders: rtReminders,
		done:        make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	if s.toolsSvc != nil && s.toolsSvc.Enabled() {
		tickSec := s.toolsCfg.ReminderTickSeconds
		if tickSec <= 0 {
			tickSec = 60
		}
		reminderTicker := time.NewTicker(time.Duration(tickSec) * time.Second)
		go func() {
			for {
				select {
				case <-reminderTicker.C:
					s.scanDueReminders()
					s.scanDueTodos()
				case <-s.done:
					reminderTicker.Stop()
					return
				}
			}
		}()
		log.Printf("[Companion] reminder tick started (every %ds)", tickSec)
	}

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
	var pets []models.Pet
	s.db.Preload("LifeState").Find(&pets)

	for _, pet := range pets {
		if s.inQuietHoursForUser(pet.UserID) {
			continue
		}
		s.scanPet(context.Background(), pet)
	}
}

func (s *Scheduler) scanPet(ctx context.Context, pet models.Pet) {
	if !pet.IsAlive || pet.LifeStage == "departed" {
		return
	}
	if !s.canSendToday(ctx, pet.ID) {
		return
	}

	state := models.LifeState{Mood: 70, Love: 60, Hungry: 30, Energy: 80}
	if pet.LifeState != nil {
		state = *pet.LifeState
	}

	bondProfile, _ := s.bond.GetOrCreate(ctx, pet.ID)

	var user models.User
	if s.db.First(&user, pet.UserID).Error != nil {
		return
	}

	trigger, memorySnippet, animation := s.pickTrigger(ctx, pet, state, bondProfile, user)
	if trigger == "" {
		return
	}
	if !user.ProactiveEnabled && !s.isFollowUpTrigger(trigger) {
		return
	}

	msg, err := s.generateMessage(ctx, pet, bondProfile, state, trigger, memorySnippet)
	if err != nil || msg == "" {
		return
	}

	if s.broadcaster != nil {
		s.broadcaster.SendProactive(user.ID, msg, animation)
	}
	s.incrementDailyCount(ctx, pet.ID)
	log.Printf("[Companion] proactive sent pet=%d trigger=%s", pet.ID, trigger)
}

func (s *Scheduler) isFollowUpTrigger(trigger triggerKind) bool {
	return trigger == triggerEmotionFollowUp || trigger == triggerEventFollowUp
}

type triggerKind string

const (
	triggerEmotionFollowUp triggerKind = "emotion_followup"
	triggerEventFollowUp   triggerKind = "event_followup"
	triggerMissYou         triggerKind = "miss_you"
	triggerMorning         triggerKind = "morning"
	triggerLifeState       triggerKind = "life_state"
)

func (s *Scheduler) pickTrigger(ctx context.Context, pet models.Pet, state models.LifeState, bondProfile models.BondProfile, user models.User) (triggerKind, string, string) {
	now := time.Now()
	hoursSince := time.Since(state.LastInteraction).Hours()

	followUp := s.cfg.FollowUpEnabled && user.FollowUpEnabled
	if bondProfile.LastMoodAt.After(now.Add(-24*time.Hour)) && emotion.IsNegativeMood(bondProfile.LastMoodTag) {
		if followUp && hoursSince > 2 {
			return triggerEmotionFollowUp, bondProfile.LastMoodTag, "concerned"
		}
	}

	var eventMem models.Memory
	err := s.db.Where("pet_id = ? AND type = ? AND content LIKE ?", pet.ID, "event", "%明天%").
		Order("created_at DESC").First(&eventMem).Error
	if err == nil && eventMem.CreatedAt.Before(now.Add(-12*time.Hour)) {
		if followUp {
			return triggerEventFollowUp, eventMem.Content, "happy"
		}
	}

	if state.Hungry > 80 || state.Energy < 20 {
		return triggerLifeState, fmt.Sprintf("hungry=%d energy=%d", state.Hungry, state.Energy), "sad"
	}

	hour := now.Hour()
	morning := s.cfg.MorningGreeting && user.MorningGreeting
	if morning && hour >= 8 && hour < 9 && hoursSince > 4 {
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

要求：自然、像伙伴关心主人，不要像通知推送；禁止散文/隐喻/诗意写法；禁止用括号描述动作，只输出对话。只输出消息正文。`,
		pet.Name, personality.Traits, lifecycle.DefaultSpeechStyle(pet.LifeStage, pet.Species),
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

func (s *Scheduler) inQuietHoursForUser(userID uint64) bool {
	start, end := 23, 8
	if len(s.cfg.QuietHours) >= 2 {
		start, end = s.cfg.QuietHours[0], s.cfg.QuietHours[1]
	}
	if userID > 0 {
		var user models.User
		if s.db.First(&user, userID).Error == nil {
			if user.QuietHoursStart > 0 || user.QuietHoursEnd > 0 {
				start = user.QuietHoursStart
				if user.QuietHoursEnd > 0 {
					end = user.QuietHoursEnd
				}
			}
		}
	}
	hour := time.Now().Hour()
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

type reminderBroadcaster interface {
	SendProactiveReminder(userID, reminderID uint64, message, animation string) bool
}

func (s *Scheduler) scanDueReminders() {
	if s.toolsSvc == nil || !s.toolsSvc.Enabled() {
		return
	}
	ctx := context.Background()
	due, err := s.toolsSvc.DueReminders(ctx, time.Now())
	if err != nil || len(due) == 0 {
		return
	}
	rb, _ := s.broadcaster.(reminderBroadcaster)
	for _, r := range due {
		var pet models.Pet
		if s.db.First(&pet, r.PetID).Error != nil {
			continue
		}
		if !pet.IsAlive || pet.LifeStage == "departed" {
			_ = s.toolsSvc.MarkReminderFired(ctx, r.ID)
			continue
		}
		var user models.User
		if s.db.First(&user, r.UserID).Error != nil {
			continue
		}

		msg := s.reminderMessage(ctx, pet, r)
		if msg == "" || s.broadcaster == nil {
			continue
		}

		if s.deliverReminder(ctx, rb, user.ID, r.ID, msg) {
			log.Printf("[Companion] reminder delivered id=%d pet=%d", r.ID, r.PetID)
		} else {
			log.Printf("[Companion] reminder pending delivery id=%d pet=%d", r.ID, r.PetID)
		}
	}
}

// deliverReminder pushes a reminder to the client. Hub (main WS + Redis queue) is preferred;
// realtime voice WS is fallback. MarkReminderFired is via hub onDelivered hook, or immediately
// when only the realtime path succeeds.
func (s *Scheduler) deliverReminder(ctx context.Context, rb reminderBroadcaster, userID, reminderID uint64, msg string) bool {
	if rb != nil && rb.SendProactiveReminder(userID, reminderID, msg, "happy") {
		return true
	}
	if s.rtReminders != nil && s.rtReminders.SendProactiveReminder(userID, reminderID, msg, "happy") {
		_ = s.toolsSvc.MarkReminderFired(ctx, reminderID)
		return true
	}
	if s.broadcaster != nil && s.broadcaster.SendProactive(userID, msg, "happy") {
		_ = s.toolsSvc.MarkReminderFired(ctx, reminderID)
		return true
	}
	return false
}

func (s *Scheduler) reminderMessage(ctx context.Context, pet models.Pet, r models.Reminder) string {
	if s.ai != nil {
		prompt := fmt.Sprintf(`你是桌宠 %s，提醒主人一件事（50字以内，口语，第一人称，适合语音朗读）。
提醒内容：%s
要求：自然亲切，像伙伴轻轻叫你，不要像系统通知；禁止用括号描述动作，只输出对话。只输出正文。`, pet.Name, r.Title)
		resp, err := s.ai.Chat(ctx, ai.ChatRequest{
			Messages:    []ai.Message{{Role: "user", Content: prompt}},
			Temperature: 0.85,
			MaxTokens:   80,
		})
		if err == nil && strings.TrimSpace(resp.Content) != "" {
			return strings.TrimSpace(resp.Content)
		}
	}
	return fmt.Sprintf("到啦~ 记得%s哦", r.Title)
}

const todoNotifiedPrefix = "mochi:todo:notified:"

func (s *Scheduler) scanDueTodos() {
	if s.toolsSvc == nil || !s.toolsSvc.Enabled() || s.broadcaster == nil {
		return
	}
	ctx := context.Background()
	due, err := s.toolsSvc.DueTodos(ctx, time.Now())
	if err != nil || len(due) == 0 {
		return
	}
	for _, t := range due {
		if s.rdb != nil {
			key := fmt.Sprintf("%s%d", todoNotifiedPrefix, t.ID)
			if s.rdb.Exists(ctx, key).Val() > 0 {
				continue
			}
		}
		var pet models.Pet
		if s.db.First(&pet, t.PetID).Error != nil {
			continue
		}
		msg := fmt.Sprintf("到啦~ 记得%s哦", t.Title)
		if s.broadcaster.SendProactive(t.UserID, msg, "happy") {
			if s.rdb != nil {
				key := fmt.Sprintf("%s%d", todoNotifiedPrefix, t.ID)
				s.rdb.Set(ctx, key, "1", 7*24*time.Hour)
			}
			log.Printf("[Companion] todo due notified id=%d pet=%d", t.ID, t.PetID)
		}
	}
}
