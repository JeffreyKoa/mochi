package wellness

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

const (
	ownerActivityPrefix = "owner_activity:"
	wellnessDailyPrefix = "mochi:wellness:daily:"
	wellnessCooldownFmt = "mochi:wellness:cooldown:%d:%s"
	wellnessMealFmt     = "mochi:wellness:meal:%d:%s:%s"
)

type deferChecker func(userID uint64) bool

type Service struct {
	db      *gorm.DB
	rdb     *redis.Client
	ai      ai.AIProvider
	cfg     config.WellnessConfig
	hub     life.StateBroadcaster
	deferFn deferChecker
	done    chan struct{}
}

func NewService(db *gorm.DB, rdb *redis.Client, aiProvider ai.AIProvider, cfg config.WellnessConfig, hub life.StateBroadcaster, deferFn deferChecker) *Service {
	return &Service{
		db:      db,
		rdb:     rdb,
		ai:      aiProvider,
		cfg:     cfg,
		hub:     hub,
		deferFn: deferFn,
		done:    make(chan struct{}),
	}
}

func (s *Service) Start() {
	if !s.cfg.Enabled {
		log.Println("[Wellness] disabled")
		return
	}
	tickMin := s.cfg.TickMinutes
	if tickMin <= 0 {
		tickMin = 10
	}
	ticker := time.NewTicker(time.Duration(tickMin) * time.Minute)
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
	log.Printf("[Wellness] scanner started (every %d min)", tickMin)
}

func (s *Service) Stop() {
	close(s.done)
}

func (s *Service) SaveActivity(ctx context.Context, userID uint64, in HeartbeatInput) error {
	if s.rdb == nil {
		return fmt.Errorf("redis unavailable")
	}
	act := OwnerActivity{
		IdleSeconds:               in.IdleSeconds,
		ContinuousActiveMinutes:   in.ContinuousActiveMinutes,
		SessionActiveMinutesToday: in.SessionActiveMinutesToday,
		UpdatedAt:                 time.Now(),
	}
	raw, err := json.Marshal(act)
	if err != nil {
		return err
	}
	key := ownerActivityPrefix + fmt.Sprintf("%d", userID)
	return s.rdb.Set(ctx, key, raw, 24*time.Hour).Err()
}

func (s *Service) GetActivity(ctx context.Context, userID uint64) (*OwnerActivity, error) {
	if s.rdb == nil {
		return nil, redis.Nil
	}
	key := ownerActivityPrefix + fmt.Sprintf("%d", userID)
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var act OwnerActivity
	if err := json.Unmarshal(raw, &act); err != nil {
		return nil, err
	}
	return &act, nil
}

func (s *Service) scanAll() {
	if s.rdb == nil || s.hub == nil {
		return
	}
	var pets []models.Pet
	s.db.Preload("LifeState").Find(&pets)
	for _, pet := range pets {
		s.scanPet(context.Background(), pet)
	}
}

func (s *Service) scanPet(ctx context.Context, pet models.Pet) {
	if !pet.IsAlive || pet.LifeStage == "departed" {
		return
	}

	var user models.User
	if s.db.First(&user, pet.UserID).Error != nil {
		return
	}
	if !user.ProactiveEnabled || !user.WellnessNudgesEnabled {
		return
	}
	if s.inQuietHours(user) {
		return
	}
	if s.deferFn != nil && s.deferFn(user.ID) {
		return
	}

	act, err := s.GetActivity(ctx, user.ID)
	if err != nil {
		return
	}
	if act == nil || time.Since(act.UpdatedAt) > 15*time.Minute {
		return
	}

	maxDaily := user.WellnessDailyMax
	if maxDaily <= 0 {
		maxDaily = s.cfg.MaxDailyNudges
	}
	if maxDaily <= 0 {
		maxDaily = 2
	}
	if pet.LifeStage == "elder" || pet.LifeStage == "twilight" {
		if maxDaily > 1 {
			maxDaily = 1
		}
	}
	if !s.canSendToday(ctx, user.ID, maxDaily) {
		return
	}

	kind := s.pickNudge(ctx, pet, user, act)
	if kind == "" {
		return
	}

	bondProfile := models.BondProfile{}
	s.db.First(&bondProfile, pet.ID)

	msg, err := s.generateMessage(ctx, pet, user, bondProfile, kind, act)
	if err != nil || msg == "" {
		return
	}

	if !s.hub.SendProactive(user.ID, msg, s.animationFor(kind)) {
		return
	}

	s.recordSent(ctx, user.ID, pet.ID, kind, msg)
	log.Printf("[Wellness] nudge sent user=%d pet=%d kind=%s", user.ID, pet.ID, kind)
}

func (s *Service) pickNudge(ctx context.Context, pet models.Pet, user models.User, act *OwnerActivity) NudgeKind {
	now := time.Now()
	candidates := make([]NudgeKind, 0, 4)

	idleBreakSec := s.cfg.ActivityIdleBreakMinutes * 60
	if idleBreakSec <= 0 {
		idleBreakSec = 300
	}

	if user.WellnessRest {
		if act.ContinuousActiveMinutes >= s.overworkMinutes() && now.Hour() < s.eveningHour() {
			if !s.isTaboo(ctx, pet.ID, NudgeOverwork) && !s.onCooldown(ctx, user.ID, NudgeOverwork) {
				candidates = append(candidates, NudgeOverwork)
			}
		}
		if now.Hour() >= s.eveningHour() && act.IdleSeconds < idleBreakSec {
			if !s.isTaboo(ctx, pet.ID, NudgeOverwork) && !s.onCooldown(ctx, user.ID, NudgeOverwork) {
				candidates = append(candidates, NudgeOverwork)
			}
		}
	}

	if user.WellnessMeal {
		if meal := s.mealWindow(now, user); meal != "" && act.IdleSeconds < idleBreakSec {
			if !s.isTaboo(ctx, pet.ID, NudgeMeal) && !s.mealSentToday(ctx, user.ID, meal) {
				if s.inMealWindowWithJitter(now, user, meal) {
					candidates = append(candidates, NudgeMeal)
				}
			}
		}
	}

	if user.WellnessDrink {
		drinkMin := s.cfg.DrinkActiveMinutes
		if drinkMin <= 0 {
			drinkMin = 90
		}
		if act.ContinuousActiveMinutes >= drinkMin && now.Hour() >= 9 && now.Hour() < 21 {
			if !s.isTaboo(ctx, pet.ID, NudgeDrink) && !s.onCooldown(ctx, user.ID, NudgeDrink) {
				candidates = append(candidates, NudgeDrink)
			}
		}
	}

	if user.WellnessRest {
		restMin := s.cfg.RestActiveMinutes
		if restMin <= 0 {
			restMin = 120
		}
		if act.ContinuousActiveMinutes >= restMin {
			if !s.isTaboo(ctx, pet.ID, NudgeRest) && !s.onCooldown(ctx, user.ID, NudgeRest) {
				candidates = append(candidates, NudgeRest)
			}
		}
		if s.inAfternoonTeaWindow(now) && act.IdleSeconds < idleBreakSec {
			if !s.isTaboo(ctx, pet.ID, NudgeRest) && !s.onCooldown(ctx, user.ID, NudgeRest) {
				candidates = append(candidates, NudgeRest)
			}
		}
	}

	return s.pickByPriority(candidates)
}

func (s *Service) pickByPriority(candidates []NudgeKind) NudgeKind {
	priority := []NudgeKind{NudgeOverwork, NudgeMeal, NudgeDrink, NudgeRest}
	for _, p := range priority {
		for _, c := range candidates {
			if c == p {
				return c
			}
		}
	}
	return ""
}

func (s *Service) overworkMinutes() int {
	if s.cfg.OverworkActiveMinutes > 0 {
		return s.cfg.OverworkActiveMinutes
	}
	return 180
}

func (s *Service) eveningHour() int {
	if s.cfg.EveningRestHour > 0 {
		return s.cfg.EveningRestHour
	}
	return 22
}

func (s *Service) mealWindow(now time.Time, user models.User) string {
	lunchH := user.LunchHour
	if lunchH <= 0 {
		lunchH = 12
	}
	dinnerH := user.DinnerHour
	if dinnerH <= 0 {
		dinnerH = 18
	}
	h, m := now.Hour(), now.Minute()
	minutes := h*60 + m
	lunchStart, lunchEnd := lunchH*60, lunchH*60+60
	dinnerStart, dinnerEnd := dinnerH*60, dinnerH*60+60
	if minutes >= lunchStart && minutes < lunchEnd {
		return "lunch"
	}
	if minutes >= dinnerStart && minutes < dinnerEnd {
		return "dinner"
	}
	return ""
}

func (s *Service) inMealWindowWithJitter(now time.Time, user models.User, meal string) bool {
	lunchH := user.LunchHour
	if lunchH <= 0 {
		lunchH = 12
	}
	dinnerH := user.DinnerHour
	if dinnerH <= 0 {
		dinnerH = 18
	}
	startMin := lunchH * 60
	if meal == "dinner" {
		startMin = dinnerH * 60
	}
	elapsed := now.Hour()*60 + now.Minute() - startMin
	if elapsed < 0 {
		return false
	}
	jitter := rand.Intn(21)
	return elapsed >= jitter
}

func (s *Service) inAfternoonTeaWindow(now time.Time) bool {
	minutes := now.Hour()*60 + now.Minute()
	return minutes >= 15*60 && minutes < 15*60+30
}

func (s *Service) inQuietHours(user models.User) bool {
	start, end := 23, 8
	if user.QuietHoursStart > 0 || user.QuietHoursEnd > 0 {
		start = user.QuietHoursStart
		if user.QuietHoursEnd > 0 {
			end = user.QuietHoursEnd
		}
	}
	hour := time.Now().Hour()
	if start > end {
		return hour >= start || hour < end
	}
	return hour >= start && hour < end
}

func (s *Service) canSendToday(ctx context.Context, userID uint64, max int) bool {
	key := wellnessDailyPrefix + fmt.Sprintf("%d:%s", userID, time.Now().Format("2006-01-02"))
	count, err := s.rdb.Get(ctx, key).Int()
	if err != nil {
		return true
	}
	return count < max
}

func (s *Service) onCooldown(ctx context.Context, userID uint64, kind NudgeKind) bool {
	key := fmt.Sprintf(wellnessCooldownFmt, userID, kind)
	return s.rdb.Exists(ctx, key).Val() > 0
}

func (s *Service) mealSentToday(ctx context.Context, userID uint64, meal string) bool {
	key := fmt.Sprintf(wellnessMealFmt, userID, time.Now().Format("2006-01-02"), meal)
	return s.rdb.Exists(ctx, key).Val() > 0
}

func (s *Service) recordSent(ctx context.Context, userID, petID uint64, kind NudgeKind, msg string) {
	today := time.Now().Format("2006-01-02")
	dailyKey := wellnessDailyPrefix + fmt.Sprintf("%d:%s", userID, today)
	s.rdb.Incr(ctx, dailyKey)
	s.rdb.Expire(ctx, dailyKey, 48*time.Hour)

	cooldown := 2 * time.Hour
	if kind == NudgeMeal {
		meal := "lunch"
		if time.Now().Hour() >= 17 {
			meal = "dinner"
		}
		mealKey := fmt.Sprintf(wellnessMealFmt, userID, today, meal)
		s.rdb.Set(ctx, mealKey, "1", 48*time.Hour)
	} else {
		coolKey := fmt.Sprintf(wellnessCooldownFmt, userID, kind)
		s.rdb.Set(ctx, coolKey, "1", cooldown)
	}

	logEntry := models.WellnessNudgeLog{
		PetID:   petID,
		UserID:  userID,
		Type:    string(kind),
		Message: msg,
		SentAt:  time.Now(),
	}
	s.db.Create(&logEntry)
}

var tabooKeywords = map[NudgeKind][]string{
	NudgeDrink:    {"喝水", "饮水", "别提醒喝"},
	NudgeMeal:     {"吃饭", "用餐", "别提醒吃"},
	NudgeRest:     {"休息", "歇", "别提醒休息"},
	NudgeOverwork: {"别太累", "防过劳", "别提醒", "别硬撑"},
}

func (s *Service) isTaboo(ctx context.Context, petID uint64, kind NudgeKind) bool {
	keywords, ok := tabooKeywords[kind]
	if !ok {
		return false
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	var entries []models.UserBriefEntry
	s.db.Where("pet_id = ? AND category = ? AND created_at >= ?", petID, "taboo", since).Find(&entries)
	for _, e := range entries {
		content := strings.ToLower(e.Content)
		for _, kw := range keywords {
			if strings.Contains(content, strings.ToLower(kw)) {
				return true
			}
		}
		if strings.Contains(content, "别提醒") {
			return true
		}
	}
	return false
}

func (s *Service) animationFor(kind NudgeKind) string {
	switch kind {
	case NudgeMeal:
		return "happy"
	case NudgeOverwork, NudgeRest:
		return "concerned"
	default:
		return "idle"
	}
}

func (s *Service) generateMessage(ctx context.Context, pet models.Pet, user models.User, bond models.BondProfile, kind NudgeKind, act *OwnerActivity) (string, error) {
	if s.ai != nil && bond.RapportLevel >= 40 {
		var personality models.Personality
		_ = json.Unmarshal(pet.PersonalityJSON, &personality)

		prompt := fmt.Sprintf(`你是桌宠 %s，给主人写一条生活照护消息（50字以内，口语，第一人称，适合语音朗读）。
性格：%s，说话风格：%s
生命阶段：%s，投缘度：%d/100
照护类型：%s
主人已连续活跃约 %d 分钟

要求：像朋友轻轻关心，点明照护意图但不要说「健康提醒」；禁止散文/隐喻写法；禁止用括号描述动作，只输出对话。只输出正文。`,
			pet.Name, personality.Traits, lifecycle.DefaultSpeechStyle(pet.LifeStage, pet.Species),
			pet.LifeStage, bond.RapportLevel, kind,
			act.ContinuousActiveMinutes,
		)
		resp, err := s.ai.Chat(ctx, ai.ChatRequest{
			Messages:    []ai.Message{{Role: "user", Content: prompt}},
			Temperature: 0.85,
			MaxTokens:   100,
		})
		if err == nil && strings.TrimSpace(resp.Content) != "" {
			return strings.TrimSpace(resp.Content), nil
		}
	}
	return s.fallbackMessage(kind, pet), nil
}

func (s *Service) fallbackMessage(kind NudgeKind, pet models.Pet) string {
	young := pet.LifeStage == "newborn" || pet.LifeStage == "juvenile" || pet.LifeStage == "child"
	elder := pet.LifeStage == "elder" || pet.LifeStage == "twilight"

	switch kind {
	case NudgeDrink:
		if young {
			return "主人，喝口水水吧~我等你哦！"
		}
		if elder {
			return "记得喝口水，别一直坐着。"
		}
		return "主人，喝口水吧，我等你~"
	case NudgeMeal:
		if young {
			return "到饭点啦，别饿着肚子哦~"
		}
		if elder {
			return "该吃饭了，别空着肚子。"
		}
		return "到饭点啦，先吃饭，身体第一位哦~"
	case NudgeRest:
		if young {
			return "忙好久了，歇一会儿嘛~"
		}
		if elder {
			return "别太累，歇会儿就好。"
		}
		return "歇一会儿嘛，别一直盯着屏幕。"
	case NudgeOverwork:
		if young {
			return "今天忙好久了，别硬撑，歇歇~"
		}
		if elder {
			return "不早了，别硬撑，早点歇着。"
		}
		return "今天忙好久了，别硬撑，歇歇~"
	default:
		return "记得照顾好自己哦~"
	}
}
