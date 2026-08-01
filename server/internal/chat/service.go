package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/agent"
	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/reflection"
	"github.com/mochi-ai/server/internal/tools"
	"github.com/mochi-ai/server/internal/wellness"
	"github.com/mochi-ai/server/pkg/ai"
)

type Service struct {
	db           *gorm.DB
	ai           ai.AIProvider
	memory       *memory.Service
	life         *life.Service
	lifecycle    *lifecycle.Service
	bond         *bond.Service
	emotion      *emotion.Service
	brief        *brief.Service
	reflection   *reflection.Service
	growth       config.GrowthConfig
	toolsExec    *tools.Executor
	toolsCfg     config.ToolsConfig
	aiCfg        config.AIConfig
	runtime      *agent.Runtime
	activity     wellness.ActivityReader
}

func NewService(db *gorm.DB, aiProvider ai.AIProvider, memSvc *memory.Service, lifeSvc *life.Service, lifecycleSvc *lifecycle.Service, bondSvc *bond.Service, emotionSvc *emotion.Service, briefSvc *brief.Service, reflectionSvc *reflection.Service, growthCfg config.GrowthConfig, toolsExec *tools.Executor, toolsCfg config.ToolsConfig, aiCfg config.AIConfig) *Service {
	return &Service{
		db:           db,
		ai:           aiProvider,
		memory:       memSvc,
		life:         lifeSvc,
		lifecycle:    lifecycleSvc,
		bond:         bondSvc,
		emotion:      emotionSvc,
		brief:        briefSvc,
		reflection:   reflectionSvc,
		growth:       growthCfg,
		toolsExec:    toolsExec,
		toolsCfg:     toolsCfg,
		aiCfg:        aiCfg,
		runtime:      agent.NewRuntime(db, aiProvider, memSvc, lifeSvc, lifecycleSvc, bondSvc, emotionSvc, briefSvc, reflectionSvc, growthCfg, toolsExec, toolsCfg, aiCfg),
	}
}

func (s *Service) GetPetByUser(ctx context.Context, userID uint64) (*models.Pet, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.getPetByUser(ctx, userID)
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

// buildChatMessages, messagesForReply, postProcess and applyBondFromMessage
// have been consolidated into agent.Runtime.Turn.

func (s *Service) SetActivityReader(reader wellness.ActivityReader) {
	s.activity = reader
}

func (s *Service) activityContextForUser(ctx context.Context, userID uint64) map[string]interface{} {
	if s.activity == nil {
		return nil
	}
	act, err := s.activity.GetActivity(ctx, userID)
	if err != nil || !wellness.IsActivityFresh(act) {
		return nil
	}
	return wellness.ToActivityContext(act)
}

func (s *Service) turnMessage(ctx context.Context, userID uint64, message, triggerType string, acoustic emotion.AcousticHint, onToken func(token string)) (string, error) {
	pet, err := s.getPetByUser(ctx, userID)
	if err != nil {
		return "", err
	}

	input := agent.TurnInput{
		UserID:          userID,
		PetID:           pet.ID,
		Message:         message,
		TriggerType:     triggerType,
		ActivityContext: s.activityContextForUser(ctx, userID),
		AcousticHint:    acoustic,
	}

	out, err := s.runtime.Turn(ctx, input)
	if err != nil {
		return "", err
	}

	var fullResponse strings.Builder
	for {
		select {
		case <-ctx.Done():
			return fullResponse.String(), ctx.Err()
		case chunk, ok := <-out.ReplyStream:
			if !ok {
				return fullResponse.String(), nil
			}
			if chunk.Done {
				return fullResponse.String(), nil
			}
			fullResponse.WriteString(chunk.Content)
			if onToken != nil {
				onToken(chunk.Content)
			}
		}
	}
}

func (s *Service) StreamMessage(ctx context.Context, userID uint64, message string, onToken func(token string)) (string, error) {
	return s.turnMessage(ctx, userID, message, "user_chat", emotion.EmptyAcousticHint(), onToken)
}

func (s *Service) StreamMessageVoice(ctx context.Context, userID uint64, message string, acoustic emotion.AcousticHint, onToken func(token string)) (string, error) {
	return s.turnMessage(ctx, userID, message, "user_voice", acoustic, onToken)
}

func (s *Service) SendMessageStream(c *gin.Context, userID uint64, message string) {
	ctx := c.Request.Context()
	pet, err := s.getPetByUser(ctx, userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "pet not found"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	input := agent.TurnInput{
		UserID:          userID,
		PetID:           pet.ID,
		Message:         message,
		TriggerType:     "user_chat",
		ActivityContext: s.activityContextForUser(ctx, userID),
	}

	out, err := s.runtime.Turn(ctx, input)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Stream(func(w io.Writer) bool {
		select {
		case chunk, ok := <-out.ReplyStream:
			if !ok {
				return false
			}
			if chunk.Done {
				fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]interface{}{"content": "", "done": true}))
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]interface{}{"content": chunk.Content, "done": false}))
			return true
		case <-ctx.Done():
			return false
		}
	})
}

func (s *Service) CompleteMessage(ctx context.Context, userID uint64, message string) (string, error) {
	return s.turnMessage(ctx, userID, message, "user_chat", emotion.EmptyAcousticHint(), nil)
}

func (s *Service) Runtime() *agent.Runtime {
	return s.runtime
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
