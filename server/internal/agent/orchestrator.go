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

// PrepareChatContext 真正并行并发准备聊天上下文（情绪判定、记忆检索、画像及关系度 4 路并发获取）
func (o *Orchestrator) PrepareChatContext(ctx context.Context, petID uint64, userMsg string) AgentContext {
	var result AgentContext
	var wg sync.WaitGroup

	var emoHint emotion.Hint
	var shortHist []models.ChatMessage
	var bondProf models.BondProfile
	var userBrief string
	var memories []models.Memory

	// 1. 情绪判别 (Goroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cached := o.emotion.GetCached(ctx, petID)
		quick := emotion.QuickDetect(userMsg)
		emoHint = emotion.MergeHint(cached, quick, userMsg)
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
