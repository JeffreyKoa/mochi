package voiceprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

var ErrNotEnrolled = errors.New("voiceprint not enrolled")

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type EnrollInput struct {
	Embedding []float64 `json:"embedding"`
	Samples   int       `json:"samples"`
}

type Status struct {
	Enrolled  bool      `json:"enrolled"`
	Dim       int       `json:"dim,omitempty"`
	Samples   int       `json:"samples,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"`
	UpdatedAt *string   `json:"updated_at,omitempty"`
}

func (s *Service) Enroll(ctx context.Context, userID uint64, in EnrollInput) error {
	if len(in.Embedding) == 0 {
		return fmt.Errorf("embedding required")
	}
	if in.Samples <= 0 {
		in.Samples = 1
	}
	raw, err := json.Marshal(in.Embedding)
	if err != nil {
		return err
	}

	var existing models.Voiceprint
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		vp := models.Voiceprint{
			UserID:    userID,
			Embedding: string(raw),
			Dim:       len(in.Embedding),
			Samples:   in.Samples,
		}
		return s.db.WithContext(ctx).Create(&vp).Error
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&existing).Updates(map[string]any{
		"embedding": string(raw),
		"dim":       len(in.Embedding),
		"samples":   in.Samples,
	}).Error
}

func (s *Service) Status(ctx context.Context, userID uint64) (*Status, error) {
	var vp models.Voiceprint
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&vp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &Status{Enrolled: false}, nil
	}
	if err != nil {
		return nil, err
	}
	var emb []float64
	if err := json.Unmarshal([]byte(vp.Embedding), &emb); err != nil {
		return nil, err
	}
	ts := vp.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	return &Status{
		Enrolled:  true,
		Dim:       vp.Dim,
		Samples:   vp.Samples,
		Embedding: emb,
		UpdatedAt: &ts,
	}, nil
}

func (s *Service) Delete(ctx context.Context, userID uint64) error {
	res := s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.Voiceprint{})
	if res.Error != nil {
		return res.Error
	}
	return nil
}
