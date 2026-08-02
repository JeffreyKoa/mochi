package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/emotion"
)

func TestUpdateTopicAnchor_AskSetsOpenQuestion(t *testing.T) {
	cfg := TopicAnchorConfig{Enabled: true, StickyTurns: 3}
	out := UpdateTopicAnchor(TopicAnchor{}, "我手里这是什么？", emotion.UtteranceInsight{
		Intent:     "ask",
		Topic:      "生活",
		VisualTask: emotion.VisualTaskObject,
	}, cfg)
	if out.OpenQuestion != "我手里这是什么？" {
		t.Fatalf("open=%q", out.OpenQuestion)
	}
	if out.StickyUntilTurn != 3 {
		t.Fatalf("sticky=%d", out.StickyUntilTurn)
	}
}

func TestUpdateTopicAnchor_DecayClearsWithoutAsk(t *testing.T) {
	cfg := TopicAnchorConfig{Enabled: true, StickyTurns: 1}
	prev := TopicAnchor{
		CurrentTopic:    "工作",
		OpenQuestion:    "今天工作怎么样？",
		StickyUntilTurn: 1,
	}
	out := UpdateTopicAnchor(prev, "嗯嗯", emotion.UtteranceInsight{Intent: "chat", Topic: "工作"}, cfg)
	if out.OpenQuestion != "" || out.CurrentTopic != "" {
		t.Fatalf("expected cleared, got open=%q topic=%q sticky=%d", out.OpenQuestion, out.CurrentTopic, out.StickyUntilTurn)
	}
}

func TestUpdateTopicAnchor_TopicSwitch(t *testing.T) {
	cfg := TopicAnchorConfig{Enabled: true, StickyTurns: 3}
	prev := TopicAnchor{
		CurrentTopic:    "辨认物品",
		OpenQuestion:    "这是什么？",
		StickyUntilTurn: 2,
	}
	out := UpdateTopicAnchor(prev, "算了不问了，今天天气怎样？", emotion.UtteranceInsight{
		Intent: "ask",
		Topic:  "娱乐",
	}, cfg)
	if out.OpenQuestion != "算了不问了，今天天气怎样？" {
		t.Fatalf("open=%q", out.OpenQuestion)
	}
	if out.CurrentTopic != "娱乐" {
		t.Fatalf("topic=%q", out.CurrentTopic)
	}
}

func TestUpdateTopicAnchor_SecondTurnStaysOnTopic(t *testing.T) {
	cfg := TopicAnchorConfig{Enabled: true, StickyTurns: 3}
	prev := UpdateTopicAnchor(TopicAnchor{}, "我手里这是什么？", emotion.UtteranceInsight{
		Intent:     "ask",
		VisualTask: emotion.VisualTaskObject,
	}, cfg)
	out := UpdateTopicAnchor(prev, "你再仔细看看", emotion.UtteranceInsight{Intent: "ask", Topic: "生活"}, cfg)
	if out.OpenQuestion != "你再仔细看看" {
		t.Fatalf("open=%q", out.OpenQuestion)
	}
	if out.CurrentTopic != "辨认物品" {
		t.Fatalf("topic=%q want 辨认物品", out.CurrentTopic)
	}
	if out.StickyUntilTurn != 3 {
		t.Fatalf("sticky=%d", out.StickyUntilTurn)
	}
}
