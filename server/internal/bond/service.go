package bond

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) GetOrCreate(ctx context.Context, petID uint64) (models.BondProfile, error) {
	var bond models.BondProfile
	err := s.db.WithContext(ctx).First(&bond, "pet_id = ?", petID).Error
	if err == nil {
		return bond, nil
	}
	if err != gorm.ErrRecordNotFound {
		return bond, err
	}
	bond = models.BondProfile{
		PetID:        petID,
		RapportLevel: 20,
		TrustLevel:   15,
		SharedTopics: []byte("[]"),
		Nicknames:    []byte("{}"),
		InsideJokes:  []byte("[]"),
		LastMoodAt:   time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&bond).Error; err != nil {
		return bond, err
	}
	return bond, nil
}

func (s *Service) RecordChatTurn(ctx context.Context, petID uint64, needsEmpathy bool) error {
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}

	bond.TotalTurns++
	bond.RapportLevel = clampUint8(int(bond.RapportLevel) + 1)
	if needsEmpathy {
		bond.RapportLevel = clampUint8(int(bond.RapportLevel) + 2)
		bond.TrustLevel = clampUint8(int(bond.TrustLevel) + 3)
	}

	today := time.Now().Format("2006-01-02")
	if bond.LastChatDay != today {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		if bond.LastChatDay == yesterday {
			bond.StreakDays++
			if bond.StreakDays >= 3 && bond.StreakDays%3 == 0 {
				bond.RapportLevel = clampUint8(int(bond.RapportLevel) + 5)
				bond.TrustLevel = clampUint8(int(bond.TrustLevel) + 2)
			}
		} else if bond.LastChatDay != "" {
			bond.StreakDays = 1
		} else {
			bond.StreakDays = 1
		}
		bond.LastChatDay = today
	}

	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "rapport_level", "trust_level", "total_turns", "last_chat_day", "streak_days", "updated_at")
}

func (s *Service) UpdateMood(ctx context.Context, petID uint64, mood, intent string) error {
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	bond.LastMoodTag = mood
	bond.LastIntent = intent
	bond.LastMoodAt = time.Now()
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "last_mood_tag", "last_intent", "last_mood_at", "updated_at")
}

func (s *Service) DecayInactive(ctx context.Context, petID uint64, hoursSince float64) error {
	if hoursSince < 48 {
		return nil
	}
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	if bond.RapportLevel > 0 {
		bond.RapportLevel--
		bond.UpdatedAt = time.Now()
		return s.saveBond(ctx, bond, "rapport_level", "updated_at")
	}
	return nil
}

func (s *Service) MergeNicknames(ctx context.Context, petID uint64, userCallsPet, petCallsUser string) error {
	if userCallsPet == "" && petCallsUser == "" {
		return nil
	}
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	var nn models.BondNicknames
	_ = json.Unmarshal(bond.Nicknames, &nn)
	if userCallsPet != "" {
		nn.UserCallsPet = userCallsPet
	}
	if petCallsUser != "" {
		nn.PetCallsUser = petCallsUser
	}
	data, _ := json.Marshal(nn)
	bond.Nicknames = data
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "nicknames", "updated_at")
}

func (s *Service) AddInsideJoke(ctx context.Context, petID uint64, content string) error {
	if content == "" {
		return nil
	}
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	var jokes []models.BondInsideJoke
	_ = json.Unmarshal(bond.InsideJokes, &jokes)
	jokes = append(jokes, models.BondInsideJoke{Content: content, CreatedAt: time.Now()})
	if len(jokes) > 10 {
		jokes = jokes[len(jokes)-10:]
	}
	data, _ := json.Marshal(jokes)
	bond.InsideJokes = data
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "inside_jokes", "updated_at")
}

func (s *Service) BoostTrust(ctx context.Context, petID uint64, delta int) error {
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	bond.TrustLevel = clampUint8(int(bond.TrustLevel) + delta)
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "trust_level", "updated_at")
}

func (s *Service) AddSharedTopic(ctx context.Context, petID uint64, topic string) error {
	if topic == "" {
		return nil
	}
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	var topics []string
	_ = json.Unmarshal(bond.SharedTopics, &topics)
	for _, t := range topics {
		if t == topic {
			return nil
		}
	}
	topics = append(topics, topic)
	if len(topics) > 20 {
		topics = topics[len(topics)-20:]
	}
	data, _ := json.Marshal(topics)
	bond.SharedTopics = data
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "shared_topics", "updated_at")
}

func (s *Service) ApplyOnboardingBonus(ctx context.Context, petID uint64) error {
	bond, err := s.GetOrCreate(ctx, petID)
	if err != nil {
		return err
	}
	if bond.RapportLevel < 25 {
		bond.RapportLevel = 25
	}
	bond.UpdatedAt = time.Now()
	return s.saveBond(ctx, bond, "rapport_level", "updated_at")
}

func (s *Service) saveBond(ctx context.Context, bond models.BondProfile, fields ...string) error {
	if len(fields) == 0 {
		return s.db.WithContext(ctx).Save(&bond).Error
	}
	return s.db.WithContext(ctx).Model(&bond).Select(fields).Updates(&bond).Error
}

func ParseNicknames(raw []byte) models.BondNicknames {
	var nn models.BondNicknames
	_ = json.Unmarshal(raw, &nn)
	return nn
}

func ParseInsideJokes(raw []byte) []models.BondInsideJoke {
	var jokes []models.BondInsideJoke
	_ = json.Unmarshal(raw, &jokes)
	return jokes
}

func clampUint8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return uint8(v)
}
