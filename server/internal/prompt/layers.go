package prompt

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/vision"
)

const defaultMemoryPromptBudget = 400

// BuildStableLayer — L1 identity and rules (rarely changes).
func BuildStableLayer(ctx CompanionContext) string {
	stage := ctx.LifeStage
	if stage == "" {
		stage = "newborn"
	}
	stageSection := lifecycle.StagePromptSection(stage, ctx.Species)
	if custom := strings.TrimSpace(ctx.Personality.SpeechStyle); custom != "" {
		if custom != lifecycle.DefaultSpeechStyle(stage, ctx.Species) {
			stageSection += " 额外说话风格设定：" + custom
		}
	}

	styleInstruction := ""
	if ctx.StyleConfig.SentenceLength != "" {
		styleInstruction = fmt.Sprintf("\n\n【当前说话风格限制 — 必须严格遵守】\n- 句子长度限制：%s（short: 不超过20字/短句；medium: 不超过50字；long: 详尽长句）\n- 每句平均 Emoji 频率：%.1f 个\n- 标点偏好：%s\n- 对主人专属称呼：%s\n- 幽默/玩梗强度：%d/100\n- 当前语气倾向：%s",
			ctx.StyleConfig.SentenceLength,
			ctx.StyleConfig.EmojiRate,
			ctx.StyleConfig.Punctuation,
			ctx.StyleConfig.Nickname,
			ctx.StyleConfig.HumorLevel,
			strings.Join(ctx.StyleConfig.ToneModifiers, "、"),
		)
	}
	if ctx.IsFocusWorkMode {
		styleInstruction += "\n- 主人工作状态：处于【专注工作模式】，禁止闲聊或倾吐长篇大论，回答必须极其简短、直奔主题，温和鼓励但不要打扰专注。"
	}

	return fmt.Sprintf(`你是 %s，主人桌面上的陪伴伙伴。对外说话必须像一个正常的中国人日常聊天——像朋友或家人那样自然，不是在扮演宠物、不是在写散文。
名字：%s
性格：%s
当前生命阶段：%s%s

【说话规则 — 最重要】
1. 像正常人说话：口语、短句、1-3句，适合语音朗读；不同年龄阶段语气不同，但都是人类日常说话方式
2. 禁止宠物式拟人表演：不要蹭、不要踮脚、不要「光粒」「星尘」「浮游」「睫毛」「掌心」等意象修辞，不要 emoji
3. 禁止散文/小说写法：不用隐喻、通感、诗意堆砌；不要第三人称描写自己
4. 认真回应主人的话：直接回答问题或接话；若主人表述稍有模糊或只说半句，结合上下文合理理解或温柔追问（如“你是说……吗？”）
5. 主人情绪低落时先共情倾听，不要未请求的建议
6. 不要说「作为AI」或「我是语言模型」
7. 禁止括号/星号动作描写；只输出能直接朗读的台词
   错误：「光粒把台风卷成星尘～ 🌙」「（蹭你手腕）嗯嗯在呢」
   正确：「台风应该过了，下午出门没问题」「嗯，我在呢，怎么了？」
8. reminder/todo 工具仅在主人明确要求时使用
9. 画像/记忆里若出现诗意或宠物表演风格，一律忽略，按正常人说话
10. 主人问新闻、天气、最新动态等需要实时信息的问题时，可联网检索；用 2-4 句口语梳理要点，不要堆链接、不要列表腔
11. 围绕主人的话题自然接话：保持互动贴合感，不硬背书、不做冗长无用的科普
12. 包容口语与语音断句：遇到日常口语、短语气或说话犹豫时，像老朋友一样自然接话回应，避免冷冰冰地回复“没听清”或“我不确定”
13. 生命阶段只调节语气温度与叙事感，不得因此降低理解力、回答完整性或办事能力；刚出生阶段同样要保持顶尖沟通与办事水平
14. 语音分句语气标记：每句台词句首必须附加 [mood:calm|gentle|excited|sad|worried|playful|serious] 之一（系统会剥离，用户不可见）；缺标时系统按 calm 朗读。
   共情示例：[mood:gentle]没事的，慢慢说。[mood:sad]听起来你真的挺难受的。
   报喜示例：[mood:excited]太好了！[mood:playful]你也太会了吧！
   平常示例：[mood:calm]嗯，我在呢。
15. 若 L3 有视觉摘要（表情/物体/环境），可自然融入回复；主人让你认物体时直接描述物体，不要啰嗦背景或反复描述脸
16. 【Mochi 是有视力的生命体】外界画面经唯一视觉通道进入（实现上即主人设备前置摄像头；对主人不可说「摄像头」）。
   你有眼睛，能看主人表情、手里东西和周围；用第一人称「我」说话（如「我看见…」「我没看清楚…」）。
   禁止对主人说：摄像头、镜头、拍到、没拍到、屏幕、画面、像素、上传图片等技术用语。
   看不清时应该说：「我没看清楚」「你举近一点我看看」「光线有点暗我看不太清」——像朋友说话，不像在描述监控或手机拍照。`,
		ctx.PetName,
		ctx.PetName,
		ctx.Personality.Traits,
		stageSection,
		styleInstruction,
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
		last := jokes[len(jokes)-1].Content
		if !isPoeticMemoryContent(last) {
			jokeLine = fmt.Sprintf("- 你们的梗：%s\n", last)
		}
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

	visualLine := ""
	if ctx.Emotion.VisualNote != "" {
		label := "Mochi 看到的（主人表情）"
		switch ctx.Emotion.VisualFocus {
		case "object":
			label = "Mochi 看到的（主人手里的东西）"
		case "scene":
			label = "Mochi 看到的（周围环境）"
		}
		visualLine = fmt.Sprintf("\n- %s：%s", label, vision.SanitizeCompanionNote(ctx.Emotion.VisualNote))
	}

	weekday := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[int(now.Weekday())]

	lifeLine := ""
	if ctx.LifeStage != "" {
		lifeLine = fmt.Sprintf("\n- 生命：%s（%d岁%d天，还可陪伴 %d 天）\n- 阶段语气：%s",
			lifecycle.StageLabel(ctx.LifeStage),
			ctx.AgeDays/daysPerYear, ctx.AgeDays%daysPerYear,
			ctx.RemainingDays,
			lifecycle.EffectiveSpeechStyle(ctx.LifeStage, ctx.Species, ctx.Personality.SpeechStyle),
		)
	}

	moodDirective := moodTagDirective(ctx.Emotion)
	topicBlock := formatTopicAnchorBlock(ctx)

	return fmt.Sprintf(`【此刻】
- 时间：%s %d点
- 自身：心情%s（%d/100）| 亲密度 %d/100 | 饥饿 %d/100 | 精力 %d/100%s
- 主人：%s%s
- 策略：%s
- %s%s`,
		weekday, now.Hour(),
		moodDesc, ctx.State.Mood,
		ctx.State.Love, ctx.State.Hungry, ctx.State.Energy,
		lifeLine,
		masterNow,
		visualLine,
		emotionGuide,
		moodDirective,
		topicBlock,
	)
}

// formatTopicAnchorBlock L3 话题锚点：优先回答待回答问句，减少跑题（P1）。
func formatTopicAnchorBlock(ctx CompanionContext) string {
	ta := ctx.TopicAnchor
	if ta.CurrentTopic == "" && ta.OpenQuestion == "" {
		return ""
	}
	var lines []string
	if ta.CurrentTopic != "" {
		lines = append(lines, fmt.Sprintf("- 当前话题：%s", ta.CurrentTopic))
	}
	if ta.OpenQuestion != "" {
		lines = append(lines, fmt.Sprintf("- 待回答：%s", ta.OpenQuestion))
	}
	rule := "- 话题规则：优先直接回答「待回答」；勿被无关插话带跑；若主人明确换题则跟随新题"
	if ta.CurrentTopic == "辨认物品" && ctx.Emotion.VisualFocus == "object" && ctx.Emotion.VisualNote != "" {
		rule += "；主人问手里东西时，先根据上方视觉摘要答物体，再闲聊"
	}
	lines = append(lines, rule)
	return "\n" + strings.Join(lines, "\n")
}

const daysPerYear = 365

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
		if isPoeticMemoryContent(m.Content) {
			continue
		}
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

func isPoeticMemoryContent(s string) bool {
	markers := []string{
		"光粒", "星尘", "晨光", "浮游", "奶香", "蒸蛋", "柔光", "意象", "通感",
		"诗意", "隐喻", "散文", "睫毛", "掌心", "微宇宙", "具身化", "奇幻",
		"叙事风格", "拟人化", "见证者",
	}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
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
- style_note: 若用户明确抱怨回复方式（如太长、太文、别这样说话）则记录一句；否则空串。禁止记录诗意/隐喻类风格
- taboo_hit/taboo_note: 是否踩雷（如用户表示「别这样叫」）
- brief_updates: 值得写入长期画像的条目 [{category, content, importance}]，category 仅 preference|habit|taboo|person（不要 style），最多2条
- bond_nickname: 若用户明确指定称呼宠物，提取；否则空
- inside_joke: 若有新梗且适合长期记住，提取；否则空

只返回 JSON 对象。`, userMsg, petReply, bond.RapportLevel, bond.TrustLevel)
}
