package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/prompt"
	"github.com/mochi-ai/server/pkg/ai"
)

type chatBuildResult struct {
	pet         *models.Pet
	messages    []ai.Message
	temperature float64
	emotionHint emotion.Hint
}

type Service struct {
	db      *gorm.DB
	ai      *ai.Provider
	memory  *memory.Service
	life    *life.Service
	bond    *bond.Service
	emotion *emotion.Service
}

func NewService(db *gorm.DB, aiProvider *ai.Provider, memSvc *memory.Service, lifeSvc *life.Service, bondSvc *bond.Service, emotionSvc *emotion.Service) *Service {
	return &Service{
		db:      db,
		ai:      aiProvider,
		memory:  memSvc,
		life:    lifeSvc,
		bond:    bondSvc,
		emotion: emotionSvc,
	}
}

func (s *Service) GetPetByUser(ctx context.Context, userID uint64) (*models.Pet, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.getPetByUser(dbCtx, userID)
}

func (s *Service) getPetByUser(ctx context.Context, userID uint64) (*models.Pet, error) {
	var pet models.Pet
	err := s.db.WithContext(ctx).Preload("LifeState").Where("user_id = ?", userID).First(&pet).Error
	if err != nil {
		return nil, err
	}
	return &pet, nil
}

func (s *Service) GetHistory(ctx context.Context, petID uint64, limit int) ([]models.ChatMessage, error) {
	var messages []models.ChatMessage
	err := s.db.Where("pet_id = ?", petID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (s *Service) buildChatMessages(ctx context.Context, userID uint64, message string) (*chatBuildResult, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var pet *models.Pet
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		pet, err = s.getPetByUser(dbCtx, userID)
		if err == nil {
			break
		}
		if attempt == 0 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("pet not found")
		}
		return nil, fmt.Errorf("load pet: %w", err)
	}

	shortHistory, _ := s.memory.GetShortTerm(dbCtx, pet.ID)

	cached := s.emotion.GetCached(dbCtx, pet.ID)
	quick := emotion.QuickDetect(message)
	hint := emotion.MergeHint(cached, quick, message)

	memories, _ := s.memory.RetrieveRelevant(dbCtx, pet.ID, message, 5, hint.UserMood)
	bondProfile, _ := s.bond.GetOrCreate(dbCtx, pet.ID)

	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)

	state := models.LifeState{Mood: 70, Love: 60, Hungry: 30, Energy: 80}
	if pet.LifeState != nil {
		state = *pet.LifeState
	}

	messages := prompt.BuildCompanionPrompt(prompt.CompanionContext{
		PetName:      pet.Name,
		Personality:  personality,
		State:        state,
		Bond:         bondProfile,
		Memories:     memories,
		ShortHistory: shortHistory,
		Emotion:      hint,
	})
	messages = append(messages, ai.Message{Role: "user", Content: message})

	return &chatBuildResult{
		pet:         pet,
		messages:    messages,
		temperature: hint.Temperature,
		emotionHint: hint,
	}, nil
}

func (s *Service) StreamMessage(ctx context.Context, userID uint64, message string, onToken func(token string)) (string, error) {
	built, err := s.buildChatMessages(ctx, userID, message)
	if err != nil {
		return "", err
	}

	chunkChan, err := s.ai.ChatStream(ctx, ai.ChatRequest{
		Messages:    built.messages,
		Temperature: built.temperature,
	})
	if err != nil {
		return "", err
	}

	var fullResponse strings.Builder
	for {
		select {
		case <-ctx.Done():
			return strings.TrimSpace(fullResponse.String()), ctx.Err()
		case chunk, ok := <-chunkChan:
			if !ok {
				return strings.TrimSpace(fullResponse.String()), nil
			}
			if chunk.Done {
				reply := strings.TrimSpace(fullResponse.String())
				go s.postProcess(context.Background(), built.pet.ID, message, reply, built.emotionHint)
				return reply, nil
			}
			if chunk.Content == "" {
				continue
			}
			fullResponse.WriteString(chunk.Content)
			if onToken != nil {
				onToken(chunk.Content)
			}
		}
	}
}

func (s *Service) SendMessageStream(c *gin.Context, userID uint64, message string) {
	built, err := s.buildChatMessages(c.Request.Context(), userID, message)
	if err != nil {
		c.JSON(500, gin.H{"error": "pet not found"})
		return
	}

	ctx := c.Request.Context()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	chunkChan, err := s.ai.ChatStream(ctx, ai.ChatRequest{
		Messages:    built.messages,
		Temperature: built.temperature,
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	var fullResponse string
	c.Stream(func(w io.Writer) bool {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				return false
			}
			if chunk.Done {
				go s.postProcess(context.Background(), built.pet.ID, message, fullResponse, built.emotionHint)
				fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]interface{}{"content": "", "done": true}))
				return false
			}
			fullResponse += chunk.Content
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]interface{}{"content": chunk.Content, "done": false}))
			return true
		case <-ctx.Done():
			return false
		}
	})
}

func (s *Service) postProcess(ctx context.Context, petID uint64, userMsg, petReply string, quickHint emotion.Hint) {
	s.db.Create(&models.ChatMessage{PetID: petID, Role: "user", Content: userMsg})
	s.db.Create(&models.ChatMessage{PetID: petID, Role: "assistant", Content: petReply})

	_ = s.memory.AddShortTerm(ctx, petID, "user", userMsg)
	_ = s.memory.AddShortTerm(ctx, petID, "assistant", petReply)

	extractPrompt := prompt.MemoryExtractPrompt(userMsg, petReply)
	go s.memory.ExtractAndStore(ctx, petID, userMsg, petReply, extractPrompt)

	_ = s.bond.RecordChatTurn(ctx, petID, quickHint.NeedsEmpathy)
	_ = s.bond.UpdateMood(ctx, petID, quickHint.UserMood, quickHint.Intent)

	shortHistory, _ := s.memory.GetShortTerm(ctx, petID)
	s.emotion.ClassifyAsync(ctx, petID, userMsg, petReply, shortHistory)

	s.applyBondFromMessage(ctx, petID, userMsg, petReply)

	s.life.Interact(ctx, petID, "chat")
}

func (s *Service) applyBondFromMessage(ctx context.Context, petID uint64, userMsg, petReply string) {
	if strings.Contains(userMsg, "叫你") || strings.Contains(userMsg, "称呼") {
		for _, part := range []string{"叫你", "称呼你"} {
			if idx := strings.Index(userMsg, part); idx >= 0 {
				rest := strings.TrimSpace(userMsg[idx+len(part):])
				rest = strings.Trim(rest, "「」\"'吧了。！")
				if rest != "" && len([]rune(rest)) <= 8 {
					_ = s.bond.MergeNicknames(ctx, petID, rest, "")
				}
			}
		}
	}
	if strings.Contains(userMsg, "哈哈") && len([]rune(userMsg)) < 30 {
		_ = s.bond.AddInsideJoke(ctx, petID, userMsg)
	}
	_ = petReply
}

func (s *Service) CompleteMessage(ctx context.Context, userID uint64, message string) (string, error) {
	built, err := s.buildChatMessages(ctx, userID, message)
	if err != nil {
		return "", err
	}

	resp, err := s.ai.Chat(ctx, ai.ChatRequest{
		Messages:    built.messages,
		Temperature: built.temperature,
	})
	if err != nil {
		return "", err
	}

	reply := strings.TrimSpace(resp.Content)
	go s.postProcess(context.Background(), built.pet.ID, message, reply, built.emotionHint)
	return reply, nil
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
