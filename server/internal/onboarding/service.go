package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/models"
)

type Input struct {
	UserCallsPet string `json:"user_calls_pet"`
	PetCallsUser string `json:"pet_calls_user"`
	Traits       string `json:"traits"`
	SpeechStyle  string `json:"speech_style"`
	FirstTopic   string `json:"first_topic"`
	FirstJoke    string `json:"first_joke"`
}

type Service struct {
	db    *gorm.DB
	bond  *bond.Service
	brief *brief.Service
}

func NewService(db *gorm.DB, bondSvc *bond.Service, briefSvc *brief.Service) *Service {
	return &Service{db: db, bond: bondSvc, brief: briefSvc}
}

func (s *Service) Complete(ctx context.Context, petID uint64, in Input) error {
	var pet models.Pet
	if err := s.db.WithContext(ctx).First(&pet, petID).Error; err != nil {
		return err
	}

	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)
	if in.Traits != "" {
		personality.Traits = in.Traits
	}
	if in.SpeechStyle != "" {
		personality.SpeechStyle = in.SpeechStyle
	}
	data, _ := json.Marshal(personality)
	if err := s.db.WithContext(ctx).Model(&pet).Update("personality_json", data).Error; err != nil {
		return err
	}

	if in.UserCallsPet != "" || in.PetCallsUser != "" {
		_ = s.bond.MergeNicknames(ctx, petID, in.UserCallsPet, in.PetCallsUser)
	}
	if in.FirstTopic != "" {
		_ = s.bond.AddSharedTopic(ctx, petID, in.FirstTopic)
		if s.brief != nil {
			_ = s.brief.UpsertEntry(ctx, petID, models.UserBriefEntry{
				Category:   "habit",
				Content:    "常聊：" + in.FirstTopic,
				Importance: 0.75,
				Source:     "onboarding",
			})
		}
	}
	if in.FirstJoke != "" {
		_ = s.bond.AddInsideJoke(ctx, petID, in.FirstJoke)
	}

	_ = s.bond.ApplyOnboardingBonus(ctx, petID)

	today := time.Now().Format("2006-01-02")
	s.db.WithContext(ctx).Create(&models.Memory{
		PetID:      petID,
		Type:       "bond",
		Content:    fmt.Sprintf("我们于 %s 正式成为伙伴", today),
		Importance: 0.8,
	})

	if s.brief != nil {
		_ = s.brief.Recompile(ctx, petID)
	}
	return nil
}
