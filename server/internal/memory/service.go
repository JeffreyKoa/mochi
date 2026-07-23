package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/pkg/ai"
)

const shortTermLimit = 20
const shortTermKeyPrefix = "mochi:chat:short:"

type Service struct {
	db    *gorm.DB
	rdb   *redis.Client
	ai    *ai.Provider
	brief *brief.Service
}

func NewService(db *gorm.DB, rdb *redis.Client, aiProvider *ai.Provider, briefSvc *brief.Service) *Service {
	return &Service{db: db, rdb: rdb, ai: aiProvider, brief: briefSvc}
}

type ExtractedMemory struct {
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Importance float32 `json:"importance"`
}

func (s *Service) GetShortTerm(ctx context.Context, petID uint64) ([]models.ChatMessage, error) {
	key := fmt.Sprintf("%s%d", shortTermKeyPrefix, petID)
	items, err := s.rdb.LRange(ctx, key, 0, shortTermLimit-1).Result()
	if err != nil {
		return nil, err
	}

	var messages []models.ChatMessage
	for i := len(items) - 1; i >= 0; i-- {
		var msg models.ChatMessage
		if err := json.Unmarshal([]byte(items[i]), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *Service) AddShortTerm(ctx context.Context, petID uint64, role, content string) error {
	key := fmt.Sprintf("%s%d", shortTermKeyPrefix, petID)
	msg := models.ChatMessage{PetID: petID, Role: role, Content: content, CreatedAt: time.Now()}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	pipe := s.rdb.Pipeline()
	pipe.LPush(ctx, key, string(data))
	pipe.LTrim(ctx, key, 0, shortTermLimit-1)
	pipe.Expire(ctx, key, 7*24*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Service) RetrieveRelevant(ctx context.Context, petID uint64, query string, limit int, userMood string) ([]models.Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	var memories []models.Memory

	if emotion.IsNegativeMood(userMood) {
		s.db.Where("pet_id = ? AND type IN ?", petID, []string{"emotion", "event"}).
			Order("importance DESC, created_at DESC").
			Limit(limit).
			Find(&memories)
		if len(memories) >= limit {
			return capBondMemories(memories, limit), nil
		}
	}

	// Recent memories
	var recent []models.Memory
	s.db.Where("pet_id = ?", petID).
		Order("created_at DESC").
		Limit(5).
		Find(&recent)
	memories = mergeMemories(memories, recent)

	// Keyword match
	if query != "" {
		words := extractKeywords(query)
		if len(words) > 0 {
			var keywordMems []models.Memory
			q := s.db.Where("pet_id = ?", petID)
			for i, w := range words {
				if i == 0 {
					q = q.Where("content LIKE ?", "%"+w+"%")
				} else {
					q = q.Or("content LIKE ?", "%"+w+"%")
				}
			}
			q.Order("importance DESC").Limit(limit).Find(&keywordMems)
			memories = mergeMemories(memories, keywordMems)
		}
	}

	if len(memories) > limit {
		memories = capBondMemories(memories, limit)
	}
	return memories, nil
}

// RetrieveRelevantLegacy wraps RetrieveRelevant with neutral mood.
func (s *Service) RetrieveRelevantLegacy(ctx context.Context, petID uint64, query string, limit int) ([]models.Memory, error) {
	return s.RetrieveRelevant(ctx, petID, query, limit, "neutral")
}

func (s *Service) ExtractAndStore(ctx context.Context, petID uint64, userMsg, petReply string, extractPrompt string) error {
	if s.ai == nil {
		return nil
	}

	resp, err := s.ai.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: "user", Content: extractPrompt}},
		Temperature: 0.2,
	})
	if err != nil {
		return err
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var extracted []ExtractedMemory
	if err := json.Unmarshal([]byte(content), &extracted); err != nil {
		return nil // silently skip bad extraction
	}

	briefTouched := false
	for _, m := range extracted {
		if m.Content == "" {
			continue
		}
		memType := m.Type
		if memType == "" {
			memType = "long"
		}
		if memType == "emotion" {
			if m.Importance < 0.7 {
				m.Importance = 0.7
			}
			s.mergeRecentEmotion(ctx, petID, m.Content, m.Importance)
			continue
		}
		if memType == "bond" {
			s.applyBondMemory(ctx, petID, m.Content)
		}
		s.db.Create(&models.Memory{
			PetID:      petID,
			Type:       memType,
			Content:    m.Content,
			Importance: m.Importance,
		})
		if s.brief != nil {
			s.brief.SyncFromMemory(ctx, petID, memType, m.Content, m.Importance)
			if memType == "long" || memType == "relation" || (memType == "topic" && m.Importance >= 0.8) {
				briefTouched = true
			}
		}
	}
	if briefTouched && s.brief != nil {
		s.brief.RecompileAsync(petID)
	}
	return nil
}

func (s *Service) mergeRecentEmotion(ctx context.Context, petID uint64, content string, importance float32) {
	since := time.Now().AddDate(0, 0, -7)
	var existing models.Memory
	err := s.db.WithContext(ctx).
		Where("pet_id = ? AND type = ? AND created_at >= ?", petID, "emotion", since).
		Order("created_at DESC").
		First(&existing).Error
	if err == nil {
		existing.Content = content
		existing.Importance = importance
		s.db.WithContext(ctx).Save(&existing)
		return
	}
	s.db.WithContext(ctx).Create(&models.Memory{
		PetID:      petID,
		Type:       "emotion",
		Content:    content,
		Importance: importance,
	})
}

func (s *Service) applyBondMemory(ctx context.Context, petID uint64, content string) {
	// bond memories stored via memory table; nickname/joke extraction handled in postProcess bond updates
	_ = ctx
	_ = petID
	_ = content
}

func (s *Service) List(ctx context.Context, petID uint64) ([]models.Memory, error) {
	var memories []models.Memory
	err := s.db.Where("pet_id = ?", petID).Order("created_at DESC").Limit(100).Find(&memories).Error
	return memories, err
}

func (s *Service) Delete(ctx context.Context, petID, memoryID uint64) error {
	return s.db.Where("id = ? AND pet_id = ?", memoryID, petID).Delete(&models.Memory{}).Error
}

func extractKeywords(text string) []string {
	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "我": true, "你": true, "在": true,
		"吗": true, "呢": true, "吧": true, "啊": true, "哦": true, "嗯": true,
		"the": true, "a": true, "is": true, "are": true,
	}
	var words []string
	for _, w := range strings.Fields(text) {
		w = strings.Trim(w, "，。！？,.!?")
		if len([]rune(w)) >= 2 && !stopWords[w] {
			words = append(words, w)
		}
	}
	if len(words) > 5 {
		words = words[:5]
	}
	return words
}

func capBondMemories(memories []models.Memory, limit int) []models.Memory {
	if len(memories) <= limit {
		return memories
	}
	bondCount := 0
	var result []models.Memory
	for _, m := range memories {
		if m.Type == "bond" {
			if bondCount >= 1 {
				continue
			}
			bondCount++
		}
		result = append(result, m)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func mergeMemories(a, b []models.Memory) []models.Memory {
	seen := make(map[uint64]bool)
	var result []models.Memory
	for _, m := range append(a, b...) {
		if !seen[m.ID] {
			seen[m.ID] = true
			result = append(result, m)
		}
	}
	return result
}
