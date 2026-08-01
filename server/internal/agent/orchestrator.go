package agent

import (
	"context"
	"sync"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
)

type AgentContext struct {
	ShortHistory []models.ChatMessage
	EmotionHint  emotion.Hint
	Memories     []models.Memory
	BondProfile  models.BondProfile
	UserBrief    string
}

type Orchestrator struct {
	memory  *memory.Service
	emotion *emotion.Service
	brief   *brief.Service
	bond    *bond.Service
}

func NewOrchestrator(memSvc *memory.Service, emoSvc *emotion.Service, briefSvc *brief.Service, bondSvc *bond.Service) *Orchestrator {
	return &Orchestrator{
		memory:  memSvc,
		emotion: emoSvc,
		brief:   briefSvc,
		bond:    bondSvc,
	}
}

// PrepareChatContext 并行准备聊天上下文（情绪、记忆、画像、关系度）。
func (o *Orchestrator) PrepareChatContext(ctx context.Context, petID uint64, userMsg string, acoustic emotion.AcousticHint) AgentContext {
	var result AgentContext
	var wg sync.WaitGroup

	var emoHint emotion.Hint
	var shortHist []models.ChatMessage
	var bondProf models.BondProfile
	var userBrief string
	var memories []models.Memory

	// 1. 情绪判别（文本 + 声学融合）
	wg.Add(1)
	go func() {
		defer wg.Done()
		if o.emotion != nil {
			emoHint = o.emotion.BuildHint(ctx, petID, userMsg, acoustic)
		} else {
			emoHint = emotion.MergeAcousticHint(
				emotion.Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85},
				emotion.QuickDetect(userMsg),
				acoustic,
				0.65,
			)
		}
	}()

	// 2. 简短历史与 Bond 亲密关系 (Goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if short, err := o.memory.GetShortTerm(ctx, petID); err == nil {
			shortHist = short
		}
		if b, err := o.bond.GetOrCreate(ctx, petID); err == nil {
			bondProf = b
		}
	}()

	// 3. 用户 Brief (Goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if briefText, err := o.brief.GetCompiled(ctx, petID); err == nil {
			userBrief = briefText
		}
	}()

	// 4. 相关记忆并发检索 (Goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		quickMood := emotion.QuickDetect(userMsg).UserMood
		if mems, err := o.memory.RetrieveRelevant(ctx, petID, userMsg, 5, quickMood); err == nil {
			memories = mems
		}
	}()

	wg.Wait()

	result.EmotionHint = emoHint
	result.ShortHistory = shortHist
	result.BondProfile = bondProf
	result.UserBrief = userBrief
	result.Memories = memories

	return result
}
