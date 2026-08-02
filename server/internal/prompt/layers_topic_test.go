package prompt

import (
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/emotion"
)

func TestFormatTopicAnchorBlock_ObjectAsk(t *testing.T) {
	block := formatTopicAnchorBlock(CompanionContext{
		TopicAnchor: TopicAnchorContext{
			CurrentTopic: "辨认物品",
			OpenQuestion: "我手里这是什么？",
		},
		Emotion: emotion.Hint{
			VisualFocus: "object",
			VisualNote:  "一只白色茶杯",
		},
	})
	if !strings.Contains(block, "当前话题：辨认物品") {
		t.Fatalf("missing topic: %s", block)
	}
	if !strings.Contains(block, "待回答：我手里这是什么？") {
		t.Fatalf("missing open: %s", block)
	}
	if !strings.Contains(block, "先根据上方视觉摘要答物体") {
		t.Fatalf("missing object rule: %s", block)
	}
}

func TestFormatTopicAnchorBlock_Empty(t *testing.T) {
	if formatTopicAnchorBlock(CompanionContext{}) != "" {
		t.Fatal("expected empty block")
	}
}
