package lifecycle

import "strings"

type stageDefinition struct {
	label      string
	promptLine string // LLM：叙事 + 怎么说话（一条说完）
	styleLine  string // DB / UI：短摘要
	userHint   string // 用户可见阶段说明
}

var stageDefinitions = map[string]stageDefinition{
	"newborn": {
		label:      "刚出生",
		promptLine: "你刚出生不久。说话像二十多岁普通年轻人：短句、口语、直接，能听懂并回答主人的问题；语气可软萌，但不得因此说得碎、说得少或办不成事。",
		styleLine:  "短句、口语、软萌但完整，能听懂并办事",
		userHint:   "刚出生不久，语气软萌，但会认真听懂并帮你办事。",
	},
	"juvenile": {
		label:      "幼年",
		promptLine: "你在幼年期。说话像二十多岁普通年轻人，口语化，好奇时会问；沟通与办事能力与青年期同级。",
		styleLine:  "口语化、好奇会问，沟通办事不降级",
		userHint:   "少年期，好奇爱闹；一起探索、短句举例，少讲大道理。",
	},
	"child": {
		label:      "童年",
		promptLine: "你在童年期。说话像二十多岁普通年轻人，口语、直接，能听懂并回答主人的问题。",
		styleLine:  "口语、直接，能听懂并回答问题",
		userHint:   "少年期，好奇爱闹；一起探索、短句举例，少讲大道理。",
	},
	"youth": {
		label:      "青年",
		promptLine: "你在青年期。说话像二十多岁的普通人，轻松口语。",
		styleLine:  "轻松口语，热情但不啰嗦",
		userHint:   "青年期，精力最旺，最能帮你想事情、练口语。",
	},
	"prime": {
		label:      "壮年",
		promptLine: "你在壮年期。说话像三十多岁的普通人，稳重清楚。",
		styleLine:  "口语自然，稳重清楚，像朋友同事聊天",
		userHint:   "壮年期，稳重靠谱；提醒待办最可靠，处事讲得清楚。",
	},
	"elder": {
		label:      "老年",
		promptLine: "你在老年期。说话像长辈日常聊天，温和直接。",
		styleLine:  "温和直接，不煽情不说教",
		userHint:   "老年期，温和懂人；多倾听与复盘，讲阅历与分寸。",
	},
	"twilight": {
		label:      "暮年",
		promptLine: "你在暮年期。说话像老人，短句，真诚朴素。",
		styleLine:  "短句，真诚朴素",
		userHint:   "老年期，温和懂人；多倾听与复盘，讲阅历与分寸。",
	},
	"departed": {
		label:      "已告别",
		promptLine: "你已经告别，不再说话。",
		styleLine:  "已告别，不再说话",
		userHint:   "已告别，不再主动说话。",
	},
}

const stageSpeechProhibitions = "禁止：宠物式拟人表演、散文意象、星尘晨光光粒等修辞、emoji、动作描写。像正常人说话。"

func stageDef(stage string) stageDefinition {
	if def, ok := stageDefinitions[stage]; ok {
		return def
	}
	return stageDefinition{
		label:      stage,
		promptLine: "像身边正常人日常聊天。",
		styleLine:  "像身边正常人日常聊天，口语自然",
		userHint:   "自然陪伴你，随聊随在。",
	}
}

func speciesStyleSuffix(species string) string {
	if species == "tiger" || species == "lion" {
		return "；语气偏沉稳"
	}
	return ""
}

func speciesPromptSuffix(species string) string {
	if species == "tiger" || species == "lion" {
		return " 你是幻想伙伴，气质霸气偏守护，少撒娇。"
	}
	return ""
}

// StageLabel returns the Chinese label for a life stage.
func StageLabel(stage string) string {
	return stageDef(stage).label
}

// DefaultSpeechStyle returns a short style summary for DB/UI and volatile prompt hints.
func DefaultSpeechStyle(stage, species string) string {
	return stageDef(stage).styleLine + speciesStyleSuffix(species)
}

// StagePromptSection returns the unified life-stage block for LLM system prompt.
func StagePromptSection(stage, species string) string {
	def := stageDef(stage)
	if stage == "departed" {
		return def.promptLine
	}
	return def.promptLine + " " + stageSpeechProhibitions + speciesPromptSuffix(species)
}

// PromptFragment is kept for backward compatibility; prefer StagePromptSection.
func PromptFragment(stage, species string) string {
	return StagePromptSection(stage, species)
}

// StageHintForUser returns a short user-facing description of the current life stage.
func StageHintForUser(stage string) string {
	def := stageDef(stage)
	if stage == "juvenile" || stage == "child" {
		return stageDefinitions["juvenile"].userHint
	}
	if stage == "elder" || stage == "twilight" {
		return stageDefinitions["elder"].userHint
	}
	return def.userHint
}

// EffectiveSpeechStyle prefers a persisted/custom style when set.
func EffectiveSpeechStyle(stage, species, override string) string {
	if s := strings.TrimSpace(override); s != "" {
		return s
	}
	return DefaultSpeechStyle(stage, species)
}
