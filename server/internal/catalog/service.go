package catalog

import (
	"context"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) ListEnabled(ctx context.Context) ([]models.PetSKU, error) {
	var skus []models.PetSKU
	err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("sort_order ASC, sku_id ASC").
		Find(&skus).Error
	return skus, err
}

func (s *Service) Get(ctx context.Context, skuID string) (*models.PetSKU, error) {
	var sku models.PetSKU
	err := s.db.WithContext(ctx).First(&sku, "sku_id = ? AND enabled = ?", skuID, true).Error
	if err != nil {
		return nil, err
	}
	return &sku, nil
}
