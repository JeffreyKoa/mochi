package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/prompt"
	"github.com/mochi-ai/server/internal/reflection"
	"github.com/mochi-ai/server/internal/text"
	"github.com/mochi-ai/server/internal/tools"
	"github.com/mochi-ai/server/pkg/ai"
)

type Runtime struct {
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
	orchestrator *Orchestrator

	// Test hooks (zero value = disabled; used only from agent tests).
	testOverridePet       *models.Pet
	testAgeInfo           lifecycle.AgeInfo
	testPostProcessNotify chan struct{}
	testSkipPostProcess   bool
}

func NewRuntime(
	db *gorm.DB,
	aiProvider ai.AIProvider,
	memSvc *memory.Service,
	lifeSvc *life.Service,
	lifecycleSvc *lifecycle.Service,
	bondSvc *bond.Service,
	emoSvc *emotion.Service,
	briefSvc *brief.Service,
	reflectSvc *reflection.Service,
	growthCfg config.GrowthConfig,
	toolsExec *tools.Executor,
	toolsCfg config.ToolsConfig,
	aiCfg config.AIConfig,
) *Runtime {
	return &Runtime{
		db:           db,
		ai:           aiProvider,
		memory:       memSvc,
		life:         lifeSvc,
		lifecycle:    lifecycleSvc,
		bond:         bondSvc,
		emotion:      emoSvc,
		brief:        briefSvc,
		reflection:   reflectSvc,
		growth:       growthCfg,
		toolsExec:    toolsExec,
		toolsCfg:     toolsCfg,
		aiCfg:        aiCfg,
		orchestrator: NewOrchestrator(memSvc, emoSvc, briefSvc, bondSvc),
	}
}

func (r *Runtime) getPet(ctx context.Context, petID uint64) (*models.Pet, error) {
	if r.testOverridePet != nil {
		return r.testOverridePet, nil
	}
	var pet models.Pet
	err := r.db.WithContext(ctx).Preload("LifeState").Where("id = ?", petID).First(&pet).Error
	if err != nil {
		return nil, err
	}
	return &pet, nil
}

// Perceive runs light perception (mood, intent, topic) on user input message.
func (r *Runtime) Perceive(message string) PerceptionResult {
	qd := emotion.QuickDetect(message)
	intensity := 0.5
	if qd.NeedsEmpathy {
		intensity = 0.8
	}
	return PerceptionResult{
		UserMood:     qd.UserMood,
		Intensity:    intensity,
		Topic:        qd.Topic,
		NeedsEmpathy: qd.NeedsEmpathy,
		Intent:       qd.Intent,
	}
}

// GetPresetPersonality returns initial personality configurations for claiming/creation.
func GetPresetPersonality(role string) models.Personality {
	switch role {
	case "heal": // 治愈系小猫
		return models.Personality{
			Traits:      "温暖、治愈、高共情、倾听陪伴",
			Warmth:      95,
			Humor:       60,
			Strictness:  10,
			Energy:      40,
			Empathy:     95,
			Curiosity:   70,
			Confidence:  60,
			Logic:       50,
			Sarcasm:     5,
		}
	case "tsundere": // 傲娇猫娘
		return models.Personality{
			Traits:      "傲娇、幽默、口是心非、毒舌好面子",
			Warmth:      70,
			Humor:       90,
			Strictness:  30,
			Energy:      70,
			Empathy:     60,
			Curiosity:   80,
			Confidence:  90,
			Logic:       50,
			Sarcasm:     60,
		}
	case "butler": // AI 管家
		return models.Personality{
			Traits:      "冷静理性、逻辑清晰、讲求效率、严格认真",
			Warmth:      50,
			Humor:       40,
			Strictness:  80,
			Energy:      30,
			Empathy:     40,
			Curiosity:   50,
			Confidence:  80,
			Logic:       95,
			Sarcasm:     10,
		}
	case "mentor": // 创业导师
		return models.Personality{
			Traits:      "睿智深刻、逻辑严密、敢于挑战主人、自信果断",
			Warmth:      40,
			Humor:       50,
			Strictness:  80,
			Energy:      50,
			Empathy:     50,
			Curiosity:   70,
			Confidence:  95,
			Logic:       90,
			Sarcasm:     30,
		}
	case "dog": // 快乐狗狗
		return models.Personality{
			Traits:      "极其热情、乐观活跃、非常幽默暖心、无条件偏爱",
			Warmth:      90,
			Humor:       80,
			Strictness:  10,
			Energy:      100,
			Empathy:     80,
			Curiosity:   90,
			Confidence:  70,
			Logic:       40,
			Sarcasm:     0,
		}
	default:
		return models.Personality{
			Traits: "友好、健谈的宠物伙伴",
			Warmth: 70,
			Humor:  60,
		}
	}
}

// DecideStyle selects a reply style configuration based on pet personality, context, current FSM mood, and work mode.
func (r *Runtime) DecideStyle(
	pet *models.Pet,
	perception PerceptionResult,
	bondProfile models.BondProfile,
	emotionState string,
	isFocusWorkMode bool,
) models.StyleConfig {
	var p models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &p)

	// Default fallback for legacy pets
	if p.Warmth == 0 && p.Humor == 0 && p.Sarcasm == 0 {
		p.Warmth = 70
		p.Humor = 60
		p.Confidence = 70
		p.Logic = 50
	}

	cfg := models.StyleConfig{
		SentenceLength: "medium",
		Punctuation:    "cute",
		Nickname:       "主人",
		HumorLevel:     p.Humor,
	}

	if isFocusWorkMode {
		cfg.SentenceLength = "short"
		cfg.EmojiRate = 0.1
		cfg.ToneModifiers = []string{"专注辅助/不打扰", "极简回答"}
		nicknames := bond.ParseNicknames(bondProfile.Nicknames)
		if nicknames.PetCallsUser != "" {
			cfg.Nickname = nicknames.PetCallsUser
		}
		return cfg
	}

	// 1. Nickname mapping based on sarcasm and nicknames
	nicknames := bond.ParseNicknames(bondProfile.Nicknames)
	if nicknames.PetCallsUser != "" {
		cfg.Nickname = nicknames.PetCallsUser
	} else if p.Sarcasm >= 50 {
		cfg.Nickname = "家伙"
	}

	// 2. Sentence Length based on strictness, logic, warmth
	if p.Strictness >= 75 || p.Logic >= 80 {
		cfg.SentenceLength = "long"
	} else if p.Warmth >= 80 {
		cfg.SentenceLength = "medium"
	} else {
		cfg.SentenceLength = "short"
	}

	// 3. Emoji Rate based on warmth, energy, strictness
	emojiRate := float64(p.Warmth+p.Energy) / 200.0
	if p.Strictness >= 70 {
		emojiRate = emojiRate * 0.3
	}
	cfg.EmojiRate = emojiRate

	// 4. Tone Modifiers
	var tones []string
	if p.Warmth >= 80 {
		tones = append(tones, "温柔关心")
	}
	if p.Sarcasm >= 50 {
		tones = append(tones, "傲娇/带点小调侃")
	}
	if p.Strictness >= 70 {
		tones = append(tones, "严厉督促")
	}
	if p.Confidence >= 80 {
		tones = append(tones, "自信从容")
	}
	if p.Logic >= 80 {
		tones = append(tones, "分析有条理")
	}

	// FSM State tones
	switch emotionState {
	case "happy":
		tones = append(tones, "欢快喜悦")
	case "excited":
		tones = append(tones, "非常兴奋开心")
	case "worried":
		tones = append(tones, "流露出深深的担心与关怀")
	case "sad":
		tones = append(tones, "低落共情/有些心疼主人")
	}

	cfg.ToneModifiers = tones
	return cfg
}

// TransitionFSM calculates the next emotion state and numerical mood based on FSM rules.
// petEmpathy gates negative transitions: low-empathy pets stay emotionally detached on vent.
func TransitionFSM(oldState string, perception PerceptionResult, petEmpathy int) (string, uint8) {
	if oldState == "" {
		oldState = "calm"
	}
	newState := oldState

	isPositive := perception.Intent == "joke" || perception.UserMood == "happy"
	isNegative := perception.Intent == "vent" || perception.UserMood == "stressed"
	empathyOK := petEmpathy >= 50

	if isPositive {
		if oldState == "calm" || oldState == "sad" {
			newState = "happy"
		} else if oldState == "happy" {
			newState = "excited"
		}
	} else if isNegative && empathyOK {
		if oldState == "calm" || oldState == "happy" || oldState == "excited" {
			newState = "worried"
		} else if oldState == "worried" {
			newState = "sad"
		}
	} else {
		// Normal decay back to calm
		if oldState == "excited" {
			newState = "happy"
		} else if oldState == "happy" || oldState == "worried" || oldState == "sad" {
			newState = "calm"
		}
	}

	return newState, moodForEmotionState(newState)
}

func moodForEmotionState(state string) uint8 {
	switch state {
	case "excited":
		return 100
	case "happy":
		return 85
	case "calm":
		return 70
	case "worried":
		return 45
	case "sad":
		return 20
	default:
		return 70
	}
}

func animationForEmotionState(state string) string {
	switch state {
	case "happy", "excited":
		return "happy"
	case "worried":
		return "worried"
	case "sad":
		return "sad"
	default:
		return "idle"
	}
}

// UpdateEmotionFSM transitions FSM states for the pet and triggers real-time UI/animation pushes.
func (r *Runtime) UpdateEmotionFSM(
	ctx context.Context,
	petID uint64,
	perception PerceptionResult,
) (string, string) {
	if r.db == nil {
		return "calm", "idle"
	}

	var state models.LifeState
	if err := r.db.WithContext(ctx).First(&state, "pet_id = ?", petID).Error; err != nil {
		return "calm", "idle"
	}

	var petEmpathy int
	var pet models.Pet
	if err := r.db.WithContext(ctx).Select("personality_json").First(&pet, petID).Error; err == nil {
		var personality models.Personality
		_ = json.Unmarshal(pet.PersonalityJSON, &personality)
		petEmpathy = personality.Empathy
		if petEmpathy == 0 {
			petEmpathy = 70
		}
	}

	newState, mood := TransitionFSM(state.EmotionState, perception, petEmpathy)
	state.EmotionState = newState
	state.Mood = mood
	state.UpdatedAt = time.Now()
	r.db.WithContext(ctx).Save(&state)

	animation := animationForEmotionState(newState)
	if r.life != nil {
		r.life.BroadcastStateDirect(petID, state, animation)
	}

	return newState, animation
}

// Turn handles the full Agent turn execution, from Perceive -> Recall -> Prompt -> LLM -> PostTurn
func (r *Runtime) Turn(ctx context.Context, input TurnInput) (TurnOutput, error) {
	startTime := time.Now()
	trace := &TurnTrace{
		PetID:           input.PetID,
		UserID:          input.UserID,
		InputMessage:    input.Message,
		TriggerType:     input.TriggerType,
		ActivityContext: input.ActivityContext,
		StartTime:       startTime,
	}

	pet, err := r.getPet(ctx, input.PetID)
	if err != nil {
		trace.LLMError = fmt.Sprintf("get pet: %v", err)
		trace.DurationMs = time.Since(startTime).Milliseconds()
		return TurnOutput{Trace: trace}, err
	}

	var ageInfo lifecycle.AgeInfo
	if r.testAgeInfo.Stage != "" {
		ageInfo = r.testAgeInfo
	} else {
		var synced lifecycle.AgeInfo
		synced, _, _ = r.lifecycle.SyncPet(ctx, pet)
		ageInfo = synced
		// SyncPet may apply neglect FSM; reload life state for DecideStyle.
		if refreshed, reloadErr := r.getPet(ctx, input.PetID); reloadErr == nil {
			pet = refreshed
		}
	}
	if !ageInfo.IsAlive || ageInfo.Stage == "departed" {
		err := fmt.Errorf("pet has departed")
		trace.LLMError = err.Error()
		trace.DurationMs = time.Since(startTime).Milliseconds()
		return TurnOutput{Trace: trace}, err
	}

	// 1. Perceive
	perceiveStart := time.Now()
	perception := r.Perceive(input.Message)
	trace.Perception = perception
	trace.StepTimings.PerceiveMs = time.Since(perceiveStart).Milliseconds()

	// 2. Recall (Context Preparation)
	recallStart := time.Now()
	agentCtx := r.orchestrator.PrepareChatContext(ctx, pet.ID, input.Message, input.AcousticHint)
	trace.MemoryHitCount = len(agentCtx.Memories)
	trace.StepTimings.RecallMs = time.Since(recallStart).Milliseconds()

	// 用户说完后立即切换 FSM，在 LLM/TTS 之前推送共情动画。
	perceptionForFSM := PerceptionResult{
		UserMood:     agentCtx.EmotionHint.UserMood,
		NeedsEmpathy: agentCtx.EmotionHint.NeedsEmpathy,
		Intent:       agentCtx.EmotionHint.Intent,
		Topic:        agentCtx.EmotionHint.Topic,
	}
	if perceptionForFSM.UserMood == "" {
		perceptionForFSM.UserMood = perception.UserMood
	}
	if perceptionForFSM.Intent == "" {
		perceptionForFSM.Intent = perception.Intent
	}
	r.UpdateEmotionFSM(ctx, pet.ID, perceptionForFSM)

	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)

	// 3. DecideStyle (Phase 3 dynamic config + Focus Work Mode)
	decideStart := time.Now()

	isFocusWorkMode := FocusModeFromActivityContext(input.ActivityContext)

	emotionState := "calm"
	if pet.LifeState != nil && pet.LifeState.EmotionState != "" {
		emotionState = pet.LifeState.EmotionState
	}

	// Long time away check (neglected for 7+ days -> sad)
	if pet.LifeState != nil && !pet.LifeState.LastInteraction.IsZero() && time.Since(pet.LifeState.LastInteraction) > 7*24*time.Hour {
		emotionState = "sad"
	}

	styleCfg := r.DecideStyle(pet, perception, agentCtx.BondProfile, emotionState, isFocusWorkMode)
	trace.PersonalityDecision = PersonalityDecision{
		Strategy: styleCfg.SentenceLength,
		Notes:    strings.Join(styleCfg.ToneModifiers, ","),
	}
	trace.StepTimings.DecideMs = time.Since(decideStart).Milliseconds()

	state := models.LifeState{Mood: 70, Love: 60, Hungry: 30, Energy: 80, Health: 90, Sleep: 20, Curiosity: 50, Knowledge: 40}
	if pet.LifeState != nil {
		state = *pet.LifeState
	}

	memBudget := r.growth.MemoryPromptCharBudget
	if memBudget <= 0 {
		memBudget = 400
	}

	// 4. Build Prompt
	buildStart := time.Now()
	messages := prompt.BuildCompanionPrompt(prompt.CompanionContext{
		PetName:            pet.Name,
		Personality:        personality,
		State:              state,
		Bond:               agentCtx.BondProfile,
		UserBrief:          agentCtx.UserBrief,
		Memories:           agentCtx.Memories,
		ShortHistory:       agentCtx.ShortHistory,
		Emotion:            agentCtx.EmotionHint,
		Now:                time.Now(),
		MemoryPromptBudget: memBudget,
		LifeStage:          ageInfo.Stage,
		AgeDays:            ageInfo.AgeDays,
		RemainingDays:      ageInfo.RemainingDays,
		Species:            pet.Species,
		StyleConfig:        styleCfg,
		IsFocusWorkMode:    isFocusWorkMode,
	})
	if input.TriggerType == "system_proactive" {
		messages = append(messages, ai.Message{
			Role:    "system",
			Content: fmt.Sprintf("【系统指令 - 主动关怀】结合当前状态，以当前风格写下一句主动关心/提醒主人（无需称呼你好，字数限制在20字内，口语自然）。指令内容：%s", input.Message),
		})
	} else if input.Message != "" {
		messages = append(messages, ai.Message{Role: "user", Content: input.Message})
	}
	trace.StepTimings.BuildPromptMs = time.Since(buildStart).Milliseconds()

	// Tool execution turn
	toolRes, err := r.applyToolTurn(ctx, messages, input.Message, pet, input.UserID, agentCtx.BondProfile, agentCtx.EmotionHint)
	if err != nil {
		log.Printf("[Runtime] applyToolTurn failed: %v", err)
		toolRes = toolTurnResult{messages: messages}
	}

	outChan := make(chan ai.ChatChunk, 100)

	if toolRes.directReply != "" {
		directReply := finalizeReply(toolRes.directReply)
		go func() {
			defer close(outChan)
			for _, ch := range directReply {
				select {
				case <-ctx.Done():
					return
				default:
					outChan <- ai.ChatChunk{Content: string(ch)}
					time.Sleep(10 * time.Millisecond)
				}
			}
			outChan <- ai.ChatChunk{Done: true}

			// Post-processing execution
			postStart := time.Now()
			r.postProcess(context.Background(), pet.ID, input.Message, directReply, agentCtx.EmotionHint)
			trace.StepTimings.PostTurnMs = time.Since(postStart).Milliseconds()
			trace.DurationMs = time.Since(startTime).Milliseconds()
			r.logTrace(trace)
		}()

		return TurnOutput{
			ReplyStream: outChan,
			Trace:       trace,
		}, nil
	}

	// 5. Invoke LLM (Streaming)
	invokeStart := time.Now()
	req := r.chatAIRequest(input.Message, toolRes.messages, agentCtx.EmotionHint.Temperature)
	trace.SelectedModel = r.ai.Name()
	trace.StepTimings.InvokeLLMMs = time.Since(invokeStart).Milliseconds()

	chunkChan, err := r.ai.ChatStream(ctx, req)
	if err != nil {
		trace.LLMError = err.Error()
		trace.DurationMs = time.Since(startTime).Milliseconds()
		return TurnOutput{Trace: trace}, err
	}

	go func() {
		defer close(outChan)
		var fullResponse strings.Builder
		var sanitizer text.StreamSanitizer
		var moodStrip text.StreamMoodStripper

		for {
			select {
			case <-ctx.Done():
				reply := finalizeReply(fullResponse.String() + sanitizer.Flush() + moodStrip.Flush())
				postStart := time.Now()
				r.postProcess(context.Background(), pet.ID, input.Message, reply, agentCtx.EmotionHint)
				trace.StepTimings.PostTurnMs = time.Since(postStart).Milliseconds()
				trace.DurationMs = time.Since(startTime).Milliseconds()
				r.logTrace(trace)
				return
			case chunk, ok := <-chunkChan:
				if !ok {
					reply := finalizeReply(fullResponse.String() + sanitizer.Flush() + moodStrip.Flush())
					postStart := time.Now()
					r.postProcess(context.Background(), pet.ID, input.Message, reply, agentCtx.EmotionHint)
					trace.StepTimings.PostTurnMs = time.Since(postStart).Milliseconds()
					trace.DurationMs = time.Since(startTime).Milliseconds()
					r.logTrace(trace)
					return
				}
				if chunk.Done {
					reply := finalizeReply(fullResponse.String() + sanitizer.Flush() + moodStrip.Flush())
					postStart := time.Now()
					r.postProcess(context.Background(), pet.ID, input.Message, reply, agentCtx.EmotionHint)
					trace.StepTimings.PostTurnMs = time.Since(postStart).Milliseconds()
					outChan <- chunk
					trace.DurationMs = time.Since(startTime).Milliseconds()
					r.logTrace(trace)
					return
				}
				if chunk.Content == "" {
					continue
				}
				cleaned := sanitizer.Feed(chunk.Content)
				cleaned = moodStrip.Feed(cleaned)
				if cleaned != "" {
					fullResponse.WriteString(cleaned)
					outChan <- ai.ChatChunk{Content: cleaned}
				}
			}
		}
	}()

	return TurnOutput{
		ReplyStream: outChan,
		Trace:       trace,
	}, nil
}

func (r *Runtime) logTrace(trace *TurnTrace) {
	b, _ := json.Marshal(trace)
	log.Printf("[RuntimeTrace] %s", string(b))
}

type toolTurnResult struct {
	messages    []ai.Message
	directReply string
}

func (r *Runtime) applyToolTurn(
	ctx context.Context,
	messages []ai.Message,
	userMsg string,
	pet *models.Pet,
	userID uint64,
	bond models.BondProfile,
	hint emotion.Hint,
) (toolTurnResult, error) {
	if r.toolsExec == nil || !r.toolsExec.Enabled() {
		return toolTurnResult{messages: messages}, nil
	}
	if hint.NeedsEmpathy || hint.Intent == "vent" {
		return toolTurnResult{messages: messages}, nil
	}
	if r.aiCfg.EnableSearch && NeedsWebSearch(userMsg) {
		log.Printf("[Runtime] skip tool turn for web search query")
		return toolTurnResult{messages: messages}, nil
	}

	msgs := appendTimeContext(messages)

	maxTok := r.toolsCfg.ToolTurnMaxTokens
	if maxTok <= 0 {
		maxTok = 256
	}

	resp, err := r.ai.ChatWithTools(ctx, ai.ChatWithToolsRequest{
		Messages:    msgs,
		Tools:       tools.Registry(),
		ToolChoice:  "auto",
		Temperature: 0.2,
		MaxTokens:   maxTok,
	})
	if err != nil {
		return toolTurnResult{messages: messages}, err
	}

	if len(resp.ToolCalls) == 0 {
		needsAction := tools.NeedsToolAction(userMsg, hint)
		if needsAction {
			if hr, err := r.toolsExec.TryHeuristicCreate(ctx, tools.ExecContext{
				PetID:   pet.ID,
				UserID:  userID,
				UserMsg: userMsg,
				Bond:    bond,
			}); err != nil {
				log.Printf("[Runtime] heuristic tool create failed: %v", err)
			} else if hr != nil {
				return toolTurnResult{messages: tools.AppendHeuristicToolTurn(msgs, hr)}, nil
			}

			retryMsgs := append(msgs, ai.Message{
				Role: "user",
				Content: "【系统】主人明确要求设置提醒或待办，你必须调用 reminder_create 或 todo_add，禁止只口头答应而不调用工具。",
			})
			retryResp, retryErr := r.ai.ChatWithTools(ctx, ai.ChatWithToolsRequest{
				Messages:    retryMsgs,
				Tools:       tools.Registry(),
				ToolChoice:  "auto",
				Temperature: 0.1,
				MaxTokens:   maxTok,
			})
			if retryErr == nil && len(retryResp.ToolCalls) > 0 {
				resp = retryResp
				msgs = retryMsgs
			} else if needsAction {
				log.Printf("[Runtime] tool turn missed action for plan message: %q", userMsg)
				return toolTurnResult{messages: messages}, nil
			}
		}
		if len(resp.ToolCalls) == 0 {
			if strings.TrimSpace(resp.Content) != "" && !needsAction {
				return toolTurnResult{directReply: strings.TrimSpace(resp.Content)}, nil
			}
			return toolTurnResult{messages: messages}, nil
		}
	}

	exec := tools.ExecContext{
		PetID:   pet.ID,
		UserID:  userID,
		UserMsg: userMsg,
		Bond:    bond,
	}

	out := append([]ai.Message{}, msgs...)
	out = append(out, ai.Message{
		Role:      "assistant",
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	})

	for _, tc := range resp.ToolCalls {
		result := r.toolsExec.Run(ctx, exec, tc)
		out = append(out, ai.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    result,
		})
	}

	out = append(out, ai.Message{
		Role:    "user",
		Content: "请用1-2句口语向主人确认刚才的办事结果，不要说「已为您完成」等服务腔；禁止用括号描述动作，只输出对话。",
	})

	return toolTurnResult{messages: out}, nil
}

func (r *Runtime) chatAIRequest(userMsg string, messages []ai.Message, temperature float64) ai.ChatRequest {
	req := ai.ChatRequest{
		Messages:    messages,
		Temperature: temperature,
	}
	if !r.aiCfg.EnableSearch || !NeedsWebSearch(userMsg) {
		return req
	}
	enable := true
	req.EnableSearch = &enable
	strategy := r.aiCfg.SearchStrategy
	if strategy == "" {
		strategy = "turbo"
	}
	req.SearchOptions = &ai.SearchOptions{
		SearchStrategy: strategy,
		ForcedSearch:   true,
	}
	log.Printf("[Runtime] enable_search user_msg=%q strategy=%s", userMsg, strategy)
	return req
}

func (r *Runtime) postProcess(ctx context.Context, petID uint64, userMsg, petReply string, quickHint emotion.Hint) {
	if r.testPostProcessNotify != nil {
		select {
		case r.testPostProcessNotify <- struct{}{}:
		default:
		}
	}
	if r.testSkipPostProcess {
		return
	}

	r.db.Create(&models.ChatMessage{PetID: petID, Role: "user", Content: userMsg})
	r.db.Create(&models.ChatMessage{PetID: petID, Role: "assistant", Content: petReply})

	_ = r.memory.AddShortTerm(ctx, petID, "user", userMsg)
	_ = r.memory.AddShortTerm(ctx, petID, "assistant", petReply)

	extractPrompt := prompt.MemoryExtractPrompt(userMsg, petReply)
	go r.memory.ExtractAndStore(ctx, petID, userMsg, petReply, extractPrompt)

	_ = r.bond.RecordChatTurn(ctx, petID, quickHint.NeedsEmpathy)
	_ = r.bond.UpdateMood(ctx, petID, quickHint.UserMood, quickHint.Intent)

	shortHistory, _ := r.memory.GetShortTerm(ctx, petID)
	r.emotion.ClassifyAsync(ctx, petID, userMsg, petReply, shortHistory)

	r.applyBondFromMessage(ctx, petID, userMsg, petReply)

	bondProfile, _ := r.bond.GetOrCreate(ctx, petID)
	if r.reflection != nil {
		r.reflection.ReflectAsync(ctx, petID, userMsg, petReply, bondProfile, quickHint.NeedsEmpathy)
	}

	r.life.Interact(ctx, petID, "chat")
}

func (r *Runtime) applyBondFromMessage(ctx context.Context, petID uint64, userMsg, petReply string) {
	if strings.Contains(userMsg, "叫你") || strings.Contains(userMsg, "称呼") {
		for _, part := range []string{"叫你", "称呼你"} {
			if idx := strings.Index(userMsg, part); idx >= 0 {
				rest := strings.TrimSpace(userMsg[idx+len(part):])
				rest = strings.Trim(rest, "「」\"'吧了。！")
				if rest != "" && len([]rune(rest)) <= 8 {
					_ = r.bond.MergeNicknames(ctx, petID, rest, "")
				}
			}
		}
	}
	if strings.Contains(userMsg, "哈哈") && len([]rune(userMsg)) < 30 {
		_ = r.bond.AddInsideJoke(ctx, petID, userMsg)
	}
	_ = petReply
}

func finalizeReply(reply string) string {
	s := text.StripActionParentheticals(strings.TrimSpace(reply))
	s = text.StripMoodTags(s)
	return text.SanitizeSpokenReply(s)
}

func appendTimeContext(messages []ai.Message) []ai.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]ai.Message, len(messages))
	copy(out, messages)
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	out[0] = ai.Message{
		Role:    messages[0].Role,
		Content: messages[0].Content + fmt.Sprintf("\n\n【当前时间 UTC+8】%s", now.Format(time.RFC3339)),
	}
	return out
}
