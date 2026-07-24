package tools

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/models"
)

var ErrTooManyReminders = errors.New("too many pending reminders")

type Service struct {
	db  *gorm.DB
	cfg config.ToolsConfig
}

func NewService(db *gorm.DB, cfg config.ToolsConfig) *Service {
	return &Service{db: db, cfg: cfg}
}

func (s *Service) Enabled() bool {
	return s.cfg.Enabled
}

func (s *Service) CreateReminder(ctx context.Context, petID, userID uint64, title string, fireAt time.Time, sourceMsg string) (*models.Reminder, error) {
	var count int64
	s.db.WithContext(ctx).Model(&models.Reminder{}).
		Where("pet_id = ? AND status = ?", petID, "pending").Count(&count)
	max := s.cfg.MaxPendingReminders
	if max <= 0 {
		max = 50
	}
	if count >= int64(max) {
		return nil, ErrTooManyReminders
	}
	now := time.Now()
	r := models.Reminder{
		PetID:     petID,
		UserID:    userID,
		Title:     trimRunes(title, 256),
		FireAt:    fireAt,
		Status:    "pending",
		Source:    "chat",
		SourceMsg: trimRunes(sourceMsg, 512),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) ListReminders(ctx context.Context, petID uint64, status string, limit int) ([]models.Reminder, error) {
	if limit <= 0 {
		limit = 20
	}
	q := s.db.WithContext(ctx).Where("pet_id = ?", petID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []models.Reminder
	err := q.Order("fire_at ASC").Limit(limit).Find(&list).Error
	return list, err
}

func (s *Service) CancelReminder(ctx context.Context, petID, id uint64) error {
	res := s.db.WithContext(ctx).Model(&models.Reminder{}).
		Where("id = ? AND pet_id = ? AND status = ?", id, petID, "pending").
		Updates(map[string]interface{}{"status": "cancelled", "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) CancelReminderByTitle(ctx context.Context, petID uint64, titleMatch string) (int, error) {
	titleMatch = strings.TrimSpace(titleMatch)
	res := s.db.WithContext(ctx).Model(&models.Reminder{}).
		Where("pet_id = ? AND status = ? AND title LIKE ?", petID, "pending", "%"+titleMatch+"%").
		Updates(map[string]interface{}{"status": "cancelled", "updated_at": time.Now()})
	return int(res.RowsAffected), res.Error
}

func (s *Service) AddTodo(ctx context.Context, petID, userID uint64, title string, dueAt *time.Time) (*models.Todo, error) {
	now := time.Now()
	t := models.Todo{
		PetID:     petID,
		UserID:    userID,
		Title:     trimRunes(title, 256),
		DueAt:     dueAt,
		Done:      false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) ListTodos(ctx context.Context, petID uint64, done bool, limit int) ([]models.Todo, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []models.Todo
	err := s.db.WithContext(ctx).Where("pet_id = ? AND done = ?", petID, done).
		Order("created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (s *Service) CompleteTodo(ctx context.Context, petID, id uint64) error {
	res := s.db.WithContext(ctx).Model(&models.Todo{}).
		Where("id = ? AND pet_id = ? AND done = ?", id, petID, false).
		Updates(map[string]interface{}{"done": true, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) CompleteTodoByTitle(ctx context.Context, petID uint64, titleMatch string) (int, error) {
	titleMatch = strings.TrimSpace(titleMatch)
	res := s.db.WithContext(ctx).Model(&models.Todo{}).
		Where("pet_id = ? AND done = ? AND title LIKE ?", petID, false, "%"+titleMatch+"%").
		Updates(map[string]interface{}{"done": true, "updated_at": time.Now()})
	return int(res.RowsAffected), res.Error
}

// DueReminders returns pending reminders that should fire now.
func (s *Service) DueReminders(ctx context.Context, now time.Time) ([]models.Reminder, error) {
	var list []models.Reminder
	err := s.db.WithContext(ctx).
		Where("status = ? AND fire_at <= ?", "pending", now).
		Order("fire_at ASC").Limit(100).
		Find(&list).Error
	return list, err
}

func (s *Service) MarkReminderFired(ctx context.Context, id uint64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&models.Reminder{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": "fired", "fired_at": now, "updated_at": now}).Error
}

// DueTodos returns unfinished todos whose due_at has passed.
func (s *Service) DueTodos(ctx context.Context, now time.Time) ([]models.Todo, error) {
	var list []models.Todo
	err := s.db.WithContext(ctx).
		Where("done = ? AND due_at IS NOT NULL AND due_at <= ?", false, now).
		Order("due_at ASC").Limit(50).
		Find(&list).Error
	return list, err
}

func trimRunes(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}
