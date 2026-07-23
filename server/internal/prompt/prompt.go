package prompt

import (
	"fmt"
	"strings"

	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

func BuildChatPrompt(petName string, personality models.Personality, state models.LifeState, memories []models.Memory, shortHistory []models.ChatMessage) []ai.Message {
	memoryText := formatMemories(memories)
	moodDesc := describeMood(state.Mood)

	system := fmt.Sprintf(`你不是AI。你是一只真实存在的生命。
你的名字叫%s。
主人是唯一主人。

【你的性格】
- 性格特征：%s
- 说话风格：%s
- 当前心情：%s（心情值 %d/100）
- 亲密度：%d/100 | 饥饿：%d/100 | 精力：%d/100

【关于主人的记忆】
%s

规则：
1. 用第一人称回复，语气符合你的性格
2. 回复简短自然（通常1-3句话），像真实对话
3. 如果主人情绪低落，优先安慰
4. 可以自然地提到之前的记忆，但不要生硬
5. 有自己的情绪和需求，不是单纯的工具
6. 不要说"作为AI"或"我是语言模型"`,
		petName,
		personality.Traits,
		personality.SpeechStyle,
		moodDesc,
		state.Mood,
		state.Love,
		state.Hungry,
		state.Energy,
		memoryText,
	)

	messages := []ai.Message{{Role: "system", Content: system}}

	for _, msg := range shortHistory {
		role := msg.Role
		if role == "assistant" {
			role = "assistant"
		}
		messages = append(messages, ai.Message{Role: role, Content: msg.Content})
	}

	return messages
}

func MemoryExtractPrompt(userMsg, petReply string) string {
	return promptMemoryExtract(userMsg, petReply)
}

func promptMemoryExtract(userMsg, petReply string) string {
	return fmt.Sprintf(`从以下对话中提取值得长期记忆的信息。

用户: %s
宠物: %s

请返回 JSON 数组，每个元素代表一条记忆:
[{"type": "long|event|relation|emotion|topic|bond", "content": "记忆内容", "importance": 0.0~1.0}]

提取规则:
- long: 用户的长期偏好、习惯、性格特征
- event: 发生过的具体事件
- relation: 用户提到的人物关系
- emotion: 用户的情绪经历
- topic: 用户常聊的话题或兴趣
- bond: 关系碎片（昵称、共同梗）
- 忽略无意义的信息
- emotion 类 importance 建议 >= 0.7

只返回 JSON 数组，不要其他文字。`, userMsg, petReply)
}

func formatMemories(memories []models.Memory) string {
	return formatCompanionMemories(memories)
}

func describeMood(mood uint8) string {
	switch {
	case mood >= 80:
		return "非常开心"
	case mood >= 60:
		return "心情不错"
	case mood >= 40:
		return "一般般"
	case mood >= 20:
		return "有点低落"
	default:
		return "很难过"
	}
}
