package vision

import (
	"strings"
)

// 纯信息类 skip 关键词（topic → 子串列表）。
var skipTopicKeywords = map[string][]string{
	"weather":   {"天气", "气温", "下雨", "下雪", "forecast", "weather", "湿度", "风力"},
	"news":      {"新闻", "头条", "要闻", "资讯"},
	"time":      {"几点", "什么时间", "现在几", "日期", "星期几", "今天几号", "几点钟", "啥时候"},
	"translate": {"翻译", "英文怎么说", "什么意思", "用英语", "怎么说"},
	"calculate": {"计算", "等于多少", "加多少", "减多少", "乘", "除", "算一下"},
}

// 组合豁免短语（任一命中则不 Skip Tier-1）。
var deicticPhrases = []string{
	"看看", "帮我看", "这是什么", "适合穿", "天气预报写",
	"窗外", "手里", "镜子里", "屏幕上", "你再看", "举起来",
}

// 单字视觉/指示代词（任一命中则不 Skip）。
var deicticRunes = []rune{
	'看', '瞧', '瞅', '认', '举', '拿', '穿', '戴', '指',
	'这', '那', '此', '彼',
}

// 多字指示词。
var deicticWords = []string{
	"上面", "手里", "窗外", "画面", "照片", "屏幕", "衣服", "镜子里",
}

// HasDeicticVisualCue 指示代词/视觉动词豁免：纯信息句中含「看窗外」等仍保留视觉。
func HasDeicticVisualCue(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, p := range deicticPhrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	for _, w := range deicticWords {
		if strings.Contains(text, w) {
			return true
		}
	}
	for _, r := range text {
		for _, c := range deicticRunes {
			if r == c {
				return true
			}
		}
	}
	return false
}

// matchesSkipTopic 用户句是否命中配置的 skip topic。
func matchesSkipTopic(text string, skipTopics []string) bool {
	if text == "" || len(skipTopics) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, topic := range skipTopics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" {
			continue
		}
		for _, kw := range skipTopicKeywords[topic] {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}

// ShouldSkipTier1 Step1：纯信息意图且未命中代词豁免 → 跳过 Tier-1 VL。
func ShouldSkipTier1(text string, skipTopics []string, deicticExempt bool) bool {
	text = strings.TrimSpace(text)
	if text == "" || len(skipTopics) == 0 {
		return false
	}
	if !matchesSkipTopic(text, skipTopics) {
		return false
	}
	if deicticExempt && HasDeicticVisualCue(text) {
		return false
	}
	return true
}

// IsHardFocus 硬依赖焦点：需服务端短 Barrier 等帧 + 二次 VL。
func IsHardFocus(f Focus) bool {
	return f == FocusObject || f == FocusScene
}

// PlanNeedsHardBarrier contextual refine 是否需硬焦点 Barrier（含 face_consistency）。
func PlanNeedsHardBarrier(plan RefinePlan) bool {
	if !plan.NeedSecondVL {
		return false
	}
	if IsHardFocus(plan.Focus) {
		return true
	}
	return strings.Contains(plan.Reason, "face_consistency")
}
