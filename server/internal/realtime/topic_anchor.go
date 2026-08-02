package realtime

import (
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/vision"
)

// TopicAnchor 会话级话题锚点：跨 turn 维持当前议题与待回答问句（P1）。
type TopicAnchor struct {
	CurrentTopic    string    // 如「辨认物品」「工作」
	OpenQuestion    string    // 主人最后一个问句
	LastUserText    string    // 上一轮用户原话
	StickyUntilTurn int       // 锚定剩余 turn 数（每 turn 衰减）
	UpdatedAt       time.Time
}

// TopicAnchorConfig 话题锚点行为配置。
type TopicAnchorConfig struct {
	Enabled     bool `yaml:"enabled"`
	StickyTurns int  `yaml:"sticky_turns"`
}

// UpdateTopicAnchor 根据 classify 结果与用户原话更新锚点。
func UpdateTopicAnchor(prev TopicAnchor, userText string, insight emotion.UtteranceInsight, cfg TopicAnchorConfig) TopicAnchor {
	if !cfg.Enabled {
		return prev
	}
	sticky := cfg.StickyTurns
	if sticky <= 0 {
		sticky = 3
	}

	text := strings.TrimSpace(userText)
	out := prev
	out.LastUserText = text

	isAsk := insight.Intent == "ask" || isQuestionText(text)
	switching := isTopicSwitch(text)

	// 每 turn 衰减；无新 ask 且 sticky 用尽则清空
	if out.StickyUntilTurn > 0 {
		out.StickyUntilTurn--
	}
	anchorCleared := false
	if out.StickyUntilTurn <= 0 && !isAsk {
		out.OpenQuestion = ""
		out.CurrentTopic = ""
		anchorCleared = true
	}

	if switching {
		out.OpenQuestion = ""
		out.CurrentTopic = ""
		anchorCleared = false
	}

	if !anchorCleared || switching || isAsk {
		topic := strings.TrimSpace(insight.Topic)
		hasTopic := topic != "" && topic != "其他"
		topicChanged := hasTopic && out.CurrentTopic != "" && out.CurrentTopic != topic

		if switching || topicChanged {
			if hasTopic && !shouldKeepObjectTopic(out.CurrentTopic, text, insight) {
				out.CurrentTopic = topic
			}
		} else if hasTopic && (out.CurrentTopic == "" || isAsk) {
			if out.CurrentTopic == "" || !shouldKeepObjectTopic(out.CurrentTopic, text, insight) {
				out.CurrentTopic = topic
			}
		}
	}

	// 举物问句：锚到问句本身，便于多轮追问不跑题
	if isAsk && (insight.VisualTask == emotion.VisualTaskObject || vision.InferVisualTaskFromText(text) != "") {
		if out.CurrentTopic == "" {
			out.CurrentTopic = "辨认物品"
		}
		out.OpenQuestion = text
		out.StickyUntilTurn = sticky
	} else if isAsk {
		out.OpenQuestion = text
		out.StickyUntilTurn = sticky
	}

	out.UpdatedAt = time.Now()
	return out
}

// shouldKeepObjectTopic 辨认物品锚定期间，短追问不因 classify topic 漂移。
func shouldKeepObjectTopic(currentTopic, text string, insight emotion.UtteranceInsight) bool {
	if currentTopic != "辨认物品" {
		return false
	}
	if insight.VisualTask == emotion.VisualTaskObject || vision.InferVisualTaskFromText(text) != "" {
		return false
	}
	return true
}

func isQuestionText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	last := []rune(t)
	r := last[len(last)-1]
	return r == '?' || r == '？'
}

func isTopicSwitch(text string) bool {
	cues := []string{
		"算了", "不说这个", "换话题", "换个话题", "另外说", "对了",
		"不问了", "不管了", "不说啦", "换个问题",
	}
	for _, c := range cues {
		if strings.Contains(text, c) {
			return true
		}
	}
	return false
}
