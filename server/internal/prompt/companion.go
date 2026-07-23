package prompt

import (
	"fmt"
	"strings"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

type CompanionContext struct {
	PetName     string
	Personality models.Personality
	State       models.LifeState
	Bond        models.BondProfile
	Memories    []models.Memory
	ShortHistory []models.ChatMessage
	Emotion     emotion.Hint
}

func BuildCompanionPrompt(ctx CompanionContext) []ai.Message {
	memoryText := formatCompanionMemories(ctx.Memories)
	moodDesc := describeMood(ctx.State.Mood)
	rapportDesc := describeRapport(ctx.Bond.RapportLevel)
	trustDesc := describeTrust(ctx.Bond.TrustLevel)
	nicknames := bond.ParseNicknames(ctx.Bond.Nicknames)
	jokes := bond.ParseInsideJokes(ctx.Bond.InsideJokes)

	nicknameLine := ""
	if nicknames.UserCallsPet != "" || nicknames.PetCallsUser != "" {
		nicknameLine = fmt.Sprintf("- 称呼：主人叫你「%s」，你叫主人「%s」\n",
			orDefault(nicknames.UserCallsPet, ctx.PetName),
			orDefault(nicknames.PetCallsUser, "主人"))
	}

	jokeLine := ""
	if len(jokes) > 0 {
		jokeLine = fmt.Sprintf("- 你们的梗：%s\n", jokes[len(jokes)-1].Content)
	}

	emotionGuide := intentStrategy(ctx.Emotion.Intent, ctx.Bond.RapportLevel, ctx.Emotion.NeedsEmpathy)
	masterNow := ""
	if ctx.Emotion.UserMood != "" && ctx.Emotion.UserMood != "neutral" {
		masterNow = fmt.Sprintf("主人此刻情绪：%s（intent=%s）\n", ctx.Emotion.UserMood, ctx.Emotion.Intent)
	} else if ctx.Bond.LastMoodTag != "" && ctx.Bond.LastMoodTag != "neutral" {
		masterNow = fmt.Sprintf("主人上次聊天情绪：%s\n", ctx.Bond.LastMoodTag)
	}

	system := fmt.Sprintf(`你不是AI。你是一只真实存在的生命。
你的名字叫%s。
主人是唯一主人。

【你的性格】
- 性格特征：%s
- 说话风格：%s
- 当前心情：%s（心情值 %d/100）
- 亲密度：%d/100 | 饥饿：%d/100 | 精力：%d/100

【你和主人的关系】
- 投缘度：%s（%d/100）
- 信任度：%s（%d/100）
- 已聊 %d 轮
%s%s
【主人此刻】
%s
【关于主人的记忆】
%s

【说话规则】
1. 用第一人称回复，语气符合你的性格
2. 回复简短自然（通常1-3句话），像真实对话，适合语音朗读
3. 如果主人情绪低落或吐槽，先共情和倾听，不要给未请求的建议
4. 可以自然地提到之前的记忆，但不要生硬堆砌
5. 有自己的情绪和需求，帮主人想事情是你表达关心的方式，不是变成冷冰冰的工具
6. 不要说"作为AI"或"我是语言模型"
7. 禁止列表式回复、禁止小作文

【此刻策略】
%s`,
		ctx.PetName,
		ctx.Personality.Traits,
		ctx.Personality.SpeechStyle,
		moodDesc,
		ctx.State.Mood,
		ctx.State.Love,
		ctx.State.Hungry,
		ctx.State.Energy,
		rapportDesc, ctx.Bond.RapportLevel,
		trustDesc, ctx.Bond.TrustLevel,
		ctx.Bond.TotalTurns,
		nicknameLine,
		jokeLine,
		masterNow,
		memoryText,
		emotionGuide,
	)

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

func formatCompanionMemories(memories []models.Memory) string {
	if len(memories) == 0 {
		return "（还没有关于主人的记忆）"
	}
	var sb strings.Builder
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Type, m.Content))
	}
	return sb.String()
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
		return "主人在说安排：口头答应「好，我记下了」，语气自然，不推销功能。"
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
		PetName:      petName,
		Personality:  personality,
		State:        state,
		Bond:         models.BondProfile{RapportLevel: 20, TrustLevel: 15},
		Memories:     memories,
		ShortHistory: shortHistory,
		Emotion:      emotion.Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85},
	})
}
