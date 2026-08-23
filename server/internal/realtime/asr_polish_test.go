package realtime

import (
	"context"
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func TestRulesPolishReplacements(t *testing.T) {
	h := config.ASRPolishHomophones{
		Replacements: []config.ASRPolishPair{
			{From: "一分后", To: "一分钟后"},
		},
		PetNameAliases: []string{"卡卡"},
	}
	out := rulesPolish("一分后提醒我", "Mochi", h)
	if out != "一分钟后提醒我" {
		t.Fatalf("got %q", out)
	}
	out2 := rulesPolish("卡卡你好", "Mochi", h)
	if out2 != "Mochi你好" {
		t.Fatalf("pet alias got %q", out2)
	}
}

func TestASRPolisherDisabled(t *testing.T) {
	p := NewASRPolisher(config.RealtimeASRPolish{Enabled: false}, config.ASRPolishHomophones{}, "", "", "", "")
	r := p.Polish(context.Background(), "你好", PolishContext{})
	if r.Polished != "你好" || r.Reason != "skipped" {
		t.Fatalf("unexpected %+v", r)
	}
}

func TestASRPolisherRulesOnly(t *testing.T) {
	p := NewASRPolisher(
		config.RealtimeASRPolish{Enabled: true, Mode: "rules"},
		config.ASRPolishHomophones{
			Replacements: []config.ASRPolishPair{{From: "嗯嗯", To: ""}},
		},
		"", "", "", "",
	)
	r := p.Polish(context.Background(), "嗯嗯你好", PolishContext{})
	if !r.Changed {
		t.Fatalf("expected change %+v", r)
	}
}

func TestPolishChangeAcceptable(t *testing.T) {
	if !polishChangeAcceptable("一分钟后", "请一分钟后") {
		t.Fatal("small prefix should be ok")
	}
	if polishChangeAcceptable("好", "这是一句完全不同的人话") {
		t.Fatal("large change should reject")
	}
}
