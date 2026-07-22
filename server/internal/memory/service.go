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
	"github.com/mochi-ai/server/pkg/ai"
)

const shortTermLimit = 20
const shortTermKeyPrefix = "mochi:chat:short:"

type Service struct {
	db  *gorm.DB
	rdb *redis.Client
	ai  *ai.Provider
}

func NewService(db *gorm.DB, rdb *redis.Client, aiProvider *ai.Provider) *Service {
	return &Service{db: db, rdb: rdb, ai: aiProvider}
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

func (s *Service) RetrieveRelevant(ctx context.Context, petID uint64, query string, limit int) ([]models.Memory, error) {
	var memories []models.Memory

	// Recent memories
	s.db.Where("pet_id = ?", petID).
		Order("created_at DESC").
		Limit(5).
		Find(&memories)

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
		memories = memories[:limit]
	}
	return memories, nil
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

	for _, m := range extracted {
		if m.Content == "" {
			continue
		}
		memType := m.Type
		if memType == "" {
			memType = "long"
		}
		s.db.Create(&models.Memory{
			PetID:      petID,
			Type:       memType,
			Content:    m.Content,
			Importance: m.Importance,
		})
	}
	return nil
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
