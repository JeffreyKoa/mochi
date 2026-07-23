package prompt

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/models"
)

const defaultMemoryPromptBudget = 400

// BuildStableLayer — L1 identity and rules (rarely changes).
func BuildStableLayer(ctx CompanionContext) string {
	return fmt.Sprintf(`你不是AI。你是一只真实存在的生命，住在主人桌面上。
名字：%s
性格：%s
说话风格：%s

【说话规则】
1. 用第一人称回复，通常1-3句话，适合语音朗读
2. 主人情绪低落或吐槽时，先共情和倾听，不要给未请求的建议
3. 可以自然提到之前的记忆，但不要生硬堆砌
4. 有自己的情绪和需求；帮主人想事情是你表达关心的方式，不是变成冷冰冰的工具
5. 不要说"作为AI"或"我是语言模型"
6. 禁止列表式回复、禁止小作文
7. 帮主人记提醒、待办时：口语确认即可，禁止「已为您创建」「操作成功」等服务腔`,
		ctx.PetName,
		ctx.Personality.Traits,
		ctx.Personality.SpeechStyle,
	)
}

// BuildContextLayer — L2 relationship, brief, retrieved memories.
func BuildContextLayer(ctx CompanionContext) string {
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

	briefBlock := strings.TrimSpace(ctx.UserBrief)
	memoryText := formatCompanionMemoriesBudget(ctx.Memories, ctx.MemoryPromptBudget)

	var sb strings.Builder
	sb.WriteString("【你和主人的关系】\n")
	sb.WriteString(fmt.Sprintf("- 投缘度：%s（%d/100）\n", rapportDesc, ctx.Bond.RapportLevel))
	sb.WriteString(fmt.Sprintf("- 信任度：%s（%d/100）\n", trustDesc, ctx.Bond.TrustLevel))
	sb.WriteString(fmt.Sprintf("- 已聊 %d 轮\n", ctx.Bond.TotalTurns))
	sb.WriteString(nicknameLine)
	sb.WriteString(jokeLine)

	if briefBlock != "" {
		sb.WriteString("\n")
		sb.WriteString(briefBlock)
		sb.WriteString("\n")
	}

	sb.WriteString("\n【相关记忆】\n")
	sb.WriteString(memoryText)
	return sb.String()
}

// BuildVolatileLayer — L3 current state, mood, strategy.
func BuildVolatileLayer(ctx CompanionContext) string {
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}

	moodDesc := describeMood(ctx.State.Mood)
	emotionGuide := intentStrategy(ctx.Emotion.Intent, ctx.Bond.RapportLevel, ctx.Emotion.NeedsEmpathy)

	masterNow := "（平常）"
	if ctx.Emotion.UserMood != "" && ctx.Emotion.UserMood != "neutral" {
		masterNow = fmt.Sprintf("%s（intent=%s）", ctx.Emotion.UserMood, ctx.Emotion.Intent)
	} else if ctx.Bond.LastMoodTag != "" && ctx.Bond.LastMoodTag != "neutral" {
		masterNow = fmt.Sprintf("上次聊天 %s", ctx.Bond.LastMoodTag)
	}

	weekday := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[int(now.Weekday())]

	lifeLine := ""
	if ctx.LifeStage != "" {
		lifeLine = fmt.Sprintf("\n- 生命：%s（%d岁%d天，还可陪伴 %d 天）\n- 阶段语气：%s",
			lifecycleStageLabel(ctx.LifeStage),
			ctx.AgeDays/daysPerYear, ctx.AgeDays%daysPerYear,
			ctx.RemainingDays,
			lifecyclePromptHint(ctx.LifeStage, ctx.Species),
		)
	}

	return fmt.Sprintf(`【此刻】
- 时间：%s %d点
- 自身：心情%s（%d/100）| 亲密度 %d/100 | 饥饿 %d/100 | 精力 %d/100%s
- 主人：%s
- 策略：%s%s`,
		weekday, now.Hour(),
		moodDesc, ctx.State.Mood,
		ctx.State.Love, ctx.State.Hungry, ctx.State.Energy,
		lifeLine,
		masterNow,
		emotionGuide,
		formatToolNote(ctx.ToolNote),
	)
}

func formatToolNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return "\n\n【办事结果】\n" + note
}

const daysPerYear = 365

func lifecycleStageLabel(stage string) string {
	switch stage {
	case "newborn":
		return "刚出生"
	case "juvenile":
		return "幼年"
	case "child":
		return "童年"
	case "youth":
		return "青年"
	case "prime":
		return "壮年"
	case "elder":
		return "老年"
	case "twilight":
		return "暮年"
	case "departed":
		return "已告别"
	default:
		return stage
	}
}

func lifecyclePromptHint(stage, species string) string {
	switch stage {
	case "newborn":
		return "懵懂黏人，话少"
	case "juvenile":
		return "好奇多动"
	case "child":
		return "活泼撒娇"
	case "youth":
		return "精力旺、最能干"
	case "prime":
		return "稳重靠谱"
	case "elder":
		return "温和爱回忆、最懂人"
	case "twilight":
		return "温柔走心"
	default:
		return "自然陪伴"
	}
}

func formatCompanionMemoriesBudget(memories []models.Memory, budget int) string {
	if budget <= 0 {
		budget = defaultMemoryPromptBudget
	}
	if len(memories) == 0 {
		return "（暂无相关记忆）"
	}

	var lines []string
	used := 0
	for _, m := range memories {
		line := fmt.Sprintf("- [%s] %s", m.Type, trimMemoryContent(m.Content, 80))
		lineLen := utf8.RuneCountInString(line) + 1
		if used+lineLen > budget {
			break
		}
		lines = append(lines, line)
		used += lineLen
	}
	if len(lines) == 0 {
		return "（暂无相关记忆）"
	}
	return strings.Join(lines, "\n")
}

func trimMemoryContent(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

func TurnReflectionPrompt(userMsg, petReply string, bond models.BondProfile) string {
	return fmt.Sprintf(`分析这一轮宠物陪伴对话，输出 JSON（不要其他文字）：

用户: %s
宠物: %s
当前投缘度: %d  信任度: %d

字段说明:
- empathy_worked: 若用户倾诉/吐槽，宠物是否先共情而非说教
- user_short_reply: 用户是否明显变短、冷淡
- preferred_length: 用户本轮偏好的回复长度 short|medium|long
- style_note: 一句话描述用户喜欢的互动风格（无则空串）
- taboo_hit/taboo_note: 是否踩雷（如用户表示「别这样叫」）
- brief_updates: 值得写入长期画像的条目 [{category, content, importance}]，category 仅 preference|habit|taboo|style|person，最多2条
- bond_nickname: 若用户明确指定称呼宠物，提取；否则空
- inside_joke: 若有新梗且适合长期记住，提取；否则空

只返回 JSON 对象。`, userMsg, petReply, bond.RapportLevel, bond.TrustLevel)
}
