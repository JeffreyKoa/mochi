package faceprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

var ErrNotEnrolled = errors.New("faceprint not enrolled")

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

// Enroll 保存或更新主人人脸 embedding（客户端 ONNX 计算）。
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

	var existing models.Faceprint
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fp := models.Faceprint{
			UserID:    userID,
			Embedding: string(raw),
			Dim:       len(in.Embedding),
			Samples:   in.Samples,
		}
		return s.db.WithContext(ctx).Create(&fp).Error
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
	var fp models.Faceprint
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&fp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &Status{Enrolled: false}, nil
	}
	if err != nil {
		return nil, err
	}
	var emb []float64
	if err := json.Unmarshal([]byte(fp.Embedding), &emb); err != nil {
		return nil, err
	}
	ts := fp.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	return &Status{
		Enrolled:  true,
		Dim:       fp.Dim,
		Samples:   fp.Samples,
		Embedding: emb,
		UpdatedAt: &ts,
	}, nil
}

func (s *Service) Delete(ctx context.Context, userID uint64) error {
	res := s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.Faceprint{})
	if res.Error != nil {
		return res.Error
	}
	return nil
}
