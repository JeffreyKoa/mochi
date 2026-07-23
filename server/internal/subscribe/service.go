package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/catalog"
	"github.com/mochi-ai/server/internal/models"
)

var (
	ErrSKUNotFound    = errors.New("sku not found")
	ErrOrderNotFound  = errors.New("order not found")
	ErrOrderNotPaid   = errors.New("order not paid")
	ErrOrderClaimed   = errors.New("order already claimed")
	ErrOrderForbidden = errors.New("order forbidden")
)

type Service struct {
	db      *gorm.DB
	catalog *catalog.Service
}

func NewService(db *gorm.DB, catalogSvc *catalog.Service) *Service {
	return &Service{db: db, catalog: catalogSvc}
}

type AdoptInput struct {
	SKUId              string
	PersonalityPreset  string // clingy|calm|playful — optional, uses SKU default
	PetName            string
}

type AdoptResult struct {
	Order models.PetOrder `json:"order"`
	Pet   models.Pet      `json:"pet"`
	SKU   models.PetSKU   `json:"sku"`
}

// Adopt runs subscribe → skip payment (mark paid) → claim pet in one transaction.
func (s *Service) Adopt(ctx context.Context, userID uint64, in AdoptInput) (*AdoptResult, error) {
	sku, err := s.catalog.Get(ctx, in.SKUId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSKUNotFound
		}
		return nil, err
	}

	personality := sku.PersonalityJSON
	if len(personality) == 0 {
		personality = []byte(`{"traits":"粘人、跟屁虫","speech_style":"短句口语"}`)
	}

	var result AdoptResult
	now := time.Now()

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order := models.PetOrder{
			UserID:          userID,
			SKUId:           sku.SKUId,
			Status:          "paid",
			PersonalityJSON: personality,
			PaidAt:          &now,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		pet, err := s.claimPet(tx, userID, sku, personality, in.PetName, now)
		if err != nil {
			return err
		}

		order.Status = "claimed"
		order.PetID = &pet.ID
		order.ClaimedAt = &now
		if err := tx.Save(&order).Error; err != nil {
			return err
		}

		result = AdoptResult{Order: order, Pet: *pet, SKU: *sku}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) claimPet(tx *gorm.DB, userID uint64, sku *models.PetSKU, personality []byte, petName string, now time.Time) (*models.Pet, error) {
	var pet models.Pet
	err := tx.Where("user_id = ?", userID).First(&pet).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if petName == "" {
			petName = "Mochi"
		}
		pet = models.Pet{
			UserID:          userID,
			Name:            petName,
			PersonalityJSON: personality,
			SKUId:           sku.SKUId,
			Species:         sku.Species,
			Breed:           sku.Breed,
			BornAt:          now,
			MaxAgeYears:     sku.MaxAgeYears,
			LifeStage:       "newborn",
			IsAlive:         true,
		}
		if err := tx.Create(&pet).Error; err != nil {
			return nil, err
		}
		state := models.LifeState{
			PetID:           pet.ID,
			Mood:            70,
			Love:            60,
			Hungry:          30,
			Energy:          80,
			LastInteraction: now,
		}
		if err := tx.Create(&state).Error; err != nil {
			return nil, err
		}
		bond := models.BondProfile{
			PetID:        pet.ID,
			RapportLevel: 20,
			TrustLevel:   15,
			SharedTopics: []byte("[]"),
			Nicknames:    []byte("{}"),
			InsideJokes:  []byte("[]"),
			LastMoodAt:   now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&bond).Error; err != nil {
			return nil, err
		}
		return &pet, nil
	}
	if err != nil {
		return nil, err
	}

	// Existing pet: apply SKU (keep bond/memory/born_at)
	pet.SKUId = sku.SKUId
	pet.Species = sku.Species
	pet.Breed = sku.Breed
	pet.MaxAgeYears = sku.MaxAgeYears
	pet.PersonalityJSON = personality
	if petName != "" {
		pet.Name = petName
	}
	if err := tx.Save(&pet).Error; err != nil {
		return nil, err
	}
	return &pet, nil
}

func ParsePersonality(raw []byte) models.Personality {
	var p models.Personality
	_ = json.Unmarshal(raw, &p)
	return p
}
