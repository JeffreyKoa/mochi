package brief

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/models"
)

type Service struct {
	db  *gorm.DB
	cfg config.GrowthConfig
}

func NewService(db *gorm.DB, cfg config.GrowthConfig) *Service {
	return &Service{db: db, cfg: cfg}
}

func (s *Service) Enabled() bool {
	return s.cfg.Enabled
}

func (s *Service) CharBudget() int {
	if s.cfg.UserBriefCharBudget > 0 {
		return s.cfg.UserBriefCharBudget
	}
	return 1400
}

func (s *Service) GetCompiled(ctx context.Context, petID uint64) (string, error) {
	if !s.Enabled() {
		return "", nil
	}
	var brief models.UserBrief
	err := s.db.WithContext(ctx).First(&brief, "pet_id = ?", petID).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return brief.CompiledText, nil
}

func (s *Service) GetBrief(ctx context.Context, petID uint64) (models.UserBrief, []models.UserBriefEntry, []models.UserBriefEntry, error) {
	var brief models.UserBrief
	err := s.db.WithContext(ctx).First(&brief, "pet_id = ?", petID).Error
	if err == gorm.ErrRecordNotFound {
		return brief, nil, nil, nil
	}
	if err != nil {
		return brief, nil, nil, err
	}
	var entries []models.UserBriefEntry
	s.db.WithContext(ctx).Where("pet_id = ? AND status = ?", petID, "approved").
		Order("importance DESC, updated_at DESC").Find(&entries)

	var pending []models.UserBriefEntry
	if s.cfg.WriteApproval {
		s.db.WithContext(ctx).Where("pet_id = ? AND status = ?", petID, "pending").
			Order("created_at DESC").Find(&pending)
	}
	return brief, entries, pending, nil
}

func (s *Service) WriteApprovalEnabled() bool {
	return s.cfg.WriteApproval
}

func (s *Service) UpsertEntry(ctx context.Context, petID uint64, entry models.UserBriefEntry) error {
	if !s.Enabled() {
		return nil
	}
	if entry.Category == "" || entry.Content == "" {
		return nil
	}
	entry.Content = trimRunes(entry.Content, 256)
	if entry.Importance <= 0 {
		entry.Importance = 0.5
	}
	if entry.Source == "" {
		entry.Source = "extract"
	}

	needsApproval := s.cfg.WriteApproval && entry.Source != "onboarding" && entry.Source != "manual"
	if needsApproval {
		return s.queuePending(ctx, petID, entry)
	}
	entry.Status = "approved"
	return s.upsertApproved(ctx, petID, entry)
}

func (s *Service) queuePending(ctx context.Context, petID uint64, entry models.UserBriefEntry) error {
	now := time.Now()
	var existing models.UserBriefEntry
	err := s.db.WithContext(ctx).
		Where("pet_id = ? AND status = ? AND category = ? AND content LIKE ?", petID, "pending", entry.Category, prefixLike(entry.Content)).
		First(&existing).Error
	if err == nil {
		existing.Content = entry.Content
		if entry.Importance > existing.Importance {
			existing.Importance = entry.Importance
		}
		existing.Source = entry.Source
		existing.UpdatedAt = now
		return s.db.WithContext(ctx).Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	entry.PetID = petID
	entry.Status = "pending"
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return s.db.WithContext(ctx).Create(&entry).Error
}

func (s *Service) upsertApproved(ctx context.Context, petID uint64, entry models.UserBriefEntry) error {
	now := time.Now()
	var existing models.UserBriefEntry
	err := s.db.WithContext(ctx).
		Where("pet_id = ? AND status = ? AND category = ? AND content LIKE ?", petID, "approved", entry.Category, prefixLike(entry.Content)).
		First(&existing).Error
	if err == nil {
		existing.Content = entry.Content
		if entry.Importance > existing.Importance {
			existing.Importance = entry.Importance
		}
		existing.Source = entry.Source
		existing.UpdatedAt = now
		return s.db.WithContext(ctx).Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	entry.PetID = petID
	entry.Status = "approved"
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return s.db.WithContext(ctx).Create(&entry).Error
}

func (s *Service) ApproveEntry(ctx context.Context, petID, entryID uint64) error {
	var entry models.UserBriefEntry
	if err := s.db.WithContext(ctx).First(&entry, "id = ? AND pet_id = ? AND status = ?", entryID, petID, "pending").Error; err != nil {
		return err
	}
	entry.Status = "approved"
	entry.UpdatedAt = time.Now()
	if err := s.db.WithContext(ctx).Save(&entry).Error; err != nil {
		return err
	}
	return s.Recompile(ctx, petID)
}

func (s *Service) RejectEntry(ctx context.Context, petID, entryID uint64) error {
	res := s.db.WithContext(ctx).Where("id = ? AND pet_id = ? AND status = ?", entryID, petID, "pending").
		Delete(&models.UserBriefEntry{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) RecompileAsync(petID uint64) {
	if !s.Enabled() {
		return
	}
	go func() {
		_ = s.Recompile(context.Background(), petID)
	}()
}

func (s *Service) Recompile(ctx context.Context, petID uint64) error {
	if !s.Enabled() {
		return nil
	}

	budget := s.CharBudget()
	var entries []models.UserBriefEntry
	if err := s.db.WithContext(ctx).
		Where("pet_id = ? AND status = ?", petID, "approved").
		Order("importance DESC, updated_at DESC").
		Find(&entries).Error; err != nil {
		return err
	}

	header := "【主人画像】（策展摘要，优先相信）"
	if len(entries) == 0 {
		return s.saveCompiled(ctx, petID, "", budget)
	}

	var lines []string
	used := utf8.RuneCountInString(header)
	for _, e := range entries {
		line := fmt.Sprintf("- [%s] %s", e.Category, e.Content)
		lineLen := utf8.RuneCountInString(line) + 1
		if used+lineLen > budget {
			break
		}
		lines = append(lines, line)
		used += lineLen
	}

	text := header
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	}
	return s.saveCompiled(ctx, petID, text, budget)
}

func (s *Service) SyncFromMemory(ctx context.Context, petID uint64, memType, content string, importance float32) {
	if !s.Enabled() {
		return
	}
	var category string
	switch memType {
	case "long":
		if importance < 0.6 {
			return
		}
		category = "preference"
	case "relation":
		category = "person"
	case "topic":
		if importance < 0.8 {
			return
		}
		content = "常聊：" + content
		category = "habit"
	default:
		return
	}
	_ = s.UpsertEntry(ctx, petID, models.UserBriefEntry{
		Category:   category,
		Content:    content,
		Importance: importance,
		Source:     "extract",
	})
}

func (s *Service) saveCompiled(ctx context.Context, petID uint64, text string, budget int) error {
	now := time.Now()
	var brief models.UserBrief
	err := s.db.WithContext(ctx).First(&brief, "pet_id = ?", petID).Error
	if err == gorm.ErrRecordNotFound {
		brief = models.UserBrief{
			PetID:        petID,
			CompiledText: text,
			CompiledAt:   now,
			CharBudget:   uint16(budget),
			UpdatedAt:    now,
		}
		return s.db.WithContext(ctx).Create(&brief).Error
	}
	if err != nil {
		return err
	}
	brief.CompiledText = text
	brief.CompiledAt = now
	brief.CharBudget = uint16(budget)
	brief.UpdatedAt = now
	return s.db.WithContext(ctx).Save(&brief).Error
}

func prefixLike(content string) string {
	prefix := trimRunes(content, 16)
	if prefix == "" {
		return "%"
	}
	return prefix + "%"
}

func trimRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

// CompileEntries builds prompt text from entries (exported for tests).
func CompileEntries(entries []models.UserBriefEntry, budget int) string {
	header := "【主人画像】（策展摘要，优先相信）"
	if len(entries) == 0 {
		return ""
	}
	var lines []string
	used := utf8.RuneCountInString(header)
	for _, e := range entries {
		line := fmt.Sprintf("- [%s] %s", e.Category, e.Content)
		lineLen := utf8.RuneCountInString(line) + 1
		if used+lineLen > budget {
			break
		}
		lines = append(lines, line)
		used += lineLen
	}
	if len(lines) == 0 {
		return ""
	}
	return header + "\n" + strings.Join(lines, "\n")
}
