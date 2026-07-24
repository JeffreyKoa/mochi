package prompt

import (
	"fmt"
	"time"

	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

type CompanionContext struct {
	PetName            string
	Personality        models.Personality
	State              models.LifeState
	Bond               models.BondProfile
	UserBrief          string
	Memories           []models.Memory
	ShortHistory       []models.ChatMessage
	Emotion            emotion.Hint
	Now                time.Time
	MemoryPromptBudget int
	LifeStage          string
	AgeDays            int
	RemainingDays      int
	Species            string
}

func BuildCompanionPrompt(ctx CompanionContext) []ai.Message {
	stable := BuildStableLayer(ctx)
	contextLayer := BuildContextLayer(ctx)
	volatile := BuildVolatileLayer(ctx)
	system := stable + "\n\n" + contextLayer + "\n\n" + volatile

	messages := []ai.Message{{Role: "system", Content: system}}
	for _, msg := range ctx.ShortHistory {
		messages = append(messages, ai.Message{Role: msg.Role, Content: msg.Content})
	}
	return messages
}

func MemoryExtractPrompt(userMsg, petReply string) string {
	return fmt.Sprintf(`从以下对话中提取值得长期记忆的信息。

用户: %s
宠物: %s

请返回 JSON 数组，每个元素代表一条记忆:
[{"type": "long|event|relation|emotion|topic|bond", "content": "记忆内容", "importance": 0.0~1.0}]

提取规则:
- long: 用户的长期偏好、习惯、性格特征
- event: 发生过的具体事件（含「明天要…」类计划，便于日后关心）
- relation: 用户提到的人物关系
- emotion: 用户的情绪经历（如：最近工作压力大、对某事很焦虑）
- topic: 用户常聊的话题或兴趣
- bond: 关系碎片（昵称、共同梗、专属称呼）
- 忽略无意义的信息（如：你好、在吗）
- emotion 类 importance 建议 >= 0.7
- importance: 越重要的信息分数越高

只返回 JSON 数组，不要其他文字。`, userMsg, petReply)
}

func describeRapport(level uint8) string {
	switch {
	case level >= 80:
		return "非常投缘，像老朋友一样"
	case level >= 60:
		return "已经比较熟了，可以更随意"
	case level >= 40:
		return "渐渐熟悉中"
	default:
		return "还在互相了解"
	}
}

func describeTrust(level uint8) string {
	switch {
	case level >= 70:
		return "主人愿意对你说心里话"
	case level >= 45:
		return "信任在慢慢建立"
	default:
		return "还在建立信任"
	}
}

func intentStrategy(intent string, rapport uint8, needsEmpathy bool) string {
	if needsEmpathy {
		return "主人需要被理解：先共情，不说教，不给「建议你试试」类未请求的建议。可以问「想聊聊还是想静静」。"
	}
	switch intent {
	case "vent":
		return "主人在倾诉：倾听优先，简短回应，不要急于解决问题。"
	case "joke":
		if rapport >= 60 {
			return "轻松接梗，可以适度调侃，保持 fun。"
		}
		return "轻松接梗，保持友好。"
	case "ask":
		return "主人有问题：直接简短回答。"
	case "plan":
		return "主人在说安排：可调用 reminder/todo 工具；确认要短、像伙伴答应，不是助手播报。"
	default:
		return "自然闲聊，可以抛一个轻松话题。"
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// BuildChatPrompt kept for backward compatibility; delegates to companion builder.
func BuildChatPrompt(petName string, personality models.Personality, state models.LifeState, memories []models.Memory, shortHistory []models.ChatMessage) []ai.Message {
	return BuildCompanionPrompt(CompanionContext{
		PetName:            petName,
		Personality:        personality,
		State:              state,
		Bond:               models.BondProfile{RapportLevel: 20, TrustLevel: 15},
		Memories:           memories,
		ShortHistory:       shortHistory,
		Emotion:            emotion.Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85},
		MemoryPromptBudget: defaultMemoryPromptBudget,
	})
}
