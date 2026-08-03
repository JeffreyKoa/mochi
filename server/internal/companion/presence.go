package companion

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/agent"
	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/wellness"
)

const (
	presenceChatCooldownPrefix = "mochi:presence_chat:cooldown:"
	presenceChatDailyPrefix    = "mochi:presence_chat:daily:"
	presenceChatMutexPrefix    = "mochi:proactive:mutex:"
)

// PresenceChatInput 客户端上报的在场信号。
type PresenceChatInput struct {
	FaceScore           float64 `json:"face_score"`
	FaceMatch           bool    `json:"face_match"`
	Trigger             string  `json:"trigger"` // vision | audio
	LastInteractionHint int64   `json:"last_interaction_hint_sec,omitempty"`
}

// PresenceChatResult HTTP 响应。
type PresenceChatResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message,omitempty"`
	Animation string `json:"animation,omitempty"`
	Delivered bool   `json:"delivered,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type proactiveBroadcaster interface {
	SendProactiveWithSource(userID uint64, message, animation, source string) bool
}

// PresenceService 处理「看见主人在场 → 主动闲聊」。
type PresenceService struct {
	db          *gorm.DB
	rdb         *redis.Client
	runtime     *agent.Runtime
	activity    wellness.ActivityReader
	bond        *bond.Service
	cfg         config.CompanionConfig
	broadcaster proactiveBroadcaster
}

func NewPresenceService(
	db *gorm.DB,
	rdb *redis.Client,
	runtime *agent.Runtime,
	activity wellness.ActivityReader,
	bondSvc *bond.Service,
	cfg config.CompanionConfig,
	broadcaster life.StateBroadcaster,
) *PresenceService {
	b, _ := broadcaster.(proactiveBroadcaster)
	return &PresenceService{
		db:          db,
		rdb:         rdb,
		runtime:     runtime,
		activity:    activity,
		bond:        bondSvc,
		cfg:         cfg,
		broadcaster: b,
	}
}

// Trigger 尝试生成并推送在场闲聊。
func (s *PresenceService) Trigger(ctx context.Context, userID uint64, in PresenceChatInput) (*PresenceChatResult, error) {
	if !s.cfg.IsPresenceChatEnabled() {
		return &PresenceChatResult{OK: false, Reason: "disabled"}, nil
	}

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	if !user.ProactiveEnabled || !user.PresenceChatEnabled {
		return &PresenceChatResult{OK: false, Reason: "user_disabled"}, nil
	}

	var pet models.Pet
	if err := s.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		return nil, err
	}
	if !pet.IsAlive || pet.LifeStage == "departed" {
		return &PresenceChatResult{OK: false, Reason: "pet_unavailable"}, nil
	}

	if s.inQuietHours(user) {
		return &PresenceChatResult{OK: false, Reason: "quiet_hours"}, nil
	}

	cooldownMin := s.cfg.PresenceChatCooldownMin
	if s.inFocusWorkMode(ctx, userID) {
		cooldownMin *= 2
	}
	if cooldownMin <= 0 {
		cooldownMin = 45
	}
	if s.onCooldown(ctx, pet.ID, cooldownMin) {
		return &PresenceChatResult{OK: false, Reason: "cooldown"}, nil
	}
	if !s.canSendToday(ctx, pet.ID) {
		return &PresenceChatResult{OK: false, Reason: "daily_limit"}, nil
	}
	if s.proactiveMutexActive(ctx, userID) {
		return &PresenceChatResult{OK: false, Reason: "busy"}, nil
	}

	bondProfile, _ := s.bond.GetOrCreate(ctx, pet.ID)
	state := models.LifeState{Mood: 70, Love: 60}
	s.db.Where("pet_id = ?", pet.ID).First(&state)

	var act *wellness.OwnerActivity
	if s.activity != nil {
		act, _ = s.activity.GetActivity(ctx, userID)
	}

	msg, err := s.generateMessage(ctx, pet, bondProfile, state, in, act)
	if err != nil || strings.TrimSpace(msg) == "" {
		return &PresenceChatResult{OK: false, Reason: "generate_failed"}, err
	}

	animation := "happy"
	if bondProfile.RapportLevel >= 60 {
		animation = "idle"
	}

	delivered := false
	if s.broadcaster != nil {
		delivered = s.broadcaster.SendProactiveWithSource(userID, msg, animation, "presence_chat")
	}

	s.markSent(ctx, pet.ID, userID, cooldownMin)
	log.Printf("[Companion] presence_chat user=%d pet=%d trigger=%s delivered=%v", userID, pet.ID, in.Trigger, delivered)

	return &PresenceChatResult{
		OK:        true,
		Message:   msg,
		Animation: animation,
		Delivered: delivered,
	}, nil
}

func (s *PresenceService) generateMessage(
	ctx context.Context,
	pet models.Pet,
	bondProfile models.BondProfile,
	state models.LifeState,
	in PresenceChatInput,
	act *wellness.OwnerActivity,
) (string, error) {
	if s.runtime == nil {
		return s.fallbackMessage(pet.Name), nil
	}

	triggerNote := "摄像头确认主人在场"
	if in.Trigger == "audio" {
		triggerNote = "听到主人回到身边（声音感知）"
	}

	out, err := s.runtime.Turn(ctx, agent.TurnInput{
		UserID:          pet.UserID,
		PetID:           pet.ID,
		Message:         "",
		TriggerType:     "presence_smalltalk",
		ActivityContext: wellness.ToActivityContext(act),
	})
	if err != nil {
		return s.fallbackMessage(pet.Name), err
	}

	var replyBuilder strings.Builder
	for chunk := range out.ReplyStream {
		if chunk.Content != "" {
			replyBuilder.WriteString(chunk.Content)
		}
	}
	res := strings.TrimSpace(replyBuilder.String())
	if res == "" {
		return s.fallbackMessage(pet.Name), nil
	}
	_ = triggerNote
	_ = bondProfile
	_ = state
	return res, nil
}

func (s *PresenceService) fallbackMessage(petName string) string {
	return fmt.Sprintf("%s在这儿呢～今天过得怎么样？", petName)
}

func (s *PresenceService) inQuietHours(user models.User) bool {
	start, end := 23, 8
	if len(s.cfg.QuietHours) >= 2 {
		start, end = s.cfg.QuietHours[0], s.cfg.QuietHours[1]
	}
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

func (s *PresenceService) inFocusWorkMode(ctx context.Context, userID uint64) bool {
	if s.activity == nil {
		return false
	}
	act, err := s.activity.GetActivity(ctx, userID)
	if err != nil || !wellness.IsActivityFresh(act) {
		return false
	}
	return agent.IsFocusWorkMode(act.ActiveApp, act.ContinuousActiveMinutes)
}

func (s *PresenceService) onCooldown(ctx context.Context, petID uint64, cooldownMin int) bool {
	if s.rdb == nil {
		return false
	}
	key := fmt.Sprintf("%s%d", presenceChatCooldownPrefix, petID)
	exists, err := s.rdb.Exists(ctx, key).Result()
	return err == nil && exists > 0
}

func (s *PresenceService) canSendToday(ctx context.Context, petID uint64) bool {
	max := s.cfg.PresenceChatDailyMax
	if max <= 0 {
		max = 8
	}
	if s.rdb == nil {
		return true
	}
	key := fmt.Sprintf("%s%d:%s", presenceChatDailyPrefix, petID, time.Now().Format("2006-01-02"))
	count, err := s.rdb.Get(ctx, key).Int()
	if err != nil {
		return true
	}
	return count < max
}

func (s *PresenceService) proactiveMutexActive(ctx context.Context, userID uint64) bool {
	if s.rdb == nil {
		return false
	}
	key := fmt.Sprintf("%s%d", presenceChatMutexPrefix, userID)
	exists, err := s.rdb.Exists(ctx, key).Result()
	return err == nil && exists > 0
}

func (s *PresenceService) markSent(ctx context.Context, petID, userID uint64, cooldownMin int) {
	if s.rdb == nil {
		return
	}
	coolKey := fmt.Sprintf("%s%d", presenceChatCooldownPrefix, petID)
	_ = s.rdb.Set(ctx, coolKey, "1", time.Duration(cooldownMin)*time.Minute).Err()

	dailyKey := fmt.Sprintf("%s%d:%s", presenceChatDailyPrefix, petID, time.Now().Format("2006-01-02"))
	s.rdb.Incr(ctx, dailyKey)
	s.rdb.Expire(ctx, dailyKey, 48*time.Hour)

	mutexKey := fmt.Sprintf("%s%d", presenceChatMutexPrefix, userID)
	s.rdb.Set(ctx, mutexKey, "1", 5*time.Minute)
}
