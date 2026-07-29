package reflection

import (
	"testing"

	"github.com/mochi-ai/server/internal/models"
)

func TestIsActionableStyleFeedback(t *testing.T) {
	tests := []struct {
		note string
		want bool
	}{
		{"诗意隐喻、星尘晨光意象", false},
		{"用户嫌回复太长", true},
		{"不要小作文，要口语短句", true},
		{"偏好通感式表达", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isActionableStyleFeedback(tt.note); got != tt.want {
			t.Fatalf("note %q: got %v want %v", tt.note, got, tt.want)
		}
	}
}

func TestService_EvolvePersonality(t *testing.T) {
	initial := models.Personality{
		Warmth:     70,
		Humor:      60,
		Confidence: 70,
		Logic:      50,
		Energy:     60,
		Curiosity:  70,
		Strictness: 30,
		Empathy:    80,
		Sarcasm:    10,
	}

	// Test 1: User praises & laughs -> Humor & Confidence increase
	p1, c1 := EvolvePersonalityVector(initial, TurnReflection{}, "哈哈，太有趣了吧！")
	if !c1 || p1.Humor != 61 || p1.Confidence != 71 {
		t.Errorf("expected humor & confidence +1, got %+v (changed=%v)", p1, c1)
	}

	// Test 2: User complains about verbosity -> Energy & Warmth decrease
	p2, c2 := EvolvePersonalityVector(initial, TurnReflection{StyleNote: "用户嫌回复太长，啰嗦小作文"}, "")
	if !c2 || p2.Energy != 58 || p2.Warmth != 69 {
		t.Errorf("expected energy -2, warmth -1, got %+v (changed=%v)", p2, c2)
	}

	// Test 3: User complains about coldness -> Warmth increases, Sarcasm decreases
	p3, c3 := EvolvePersonalityVector(initial, TurnReflection{StyleNote: "太冷淡了，别这么高冷太短了"}, "")
	if !c3 || p3.Warmth != 72 || p3.Sarcasm != 8 {
		t.Errorf("expected warmth +2, sarcasm -2, got %+v (changed=%v)", p3, c3)
	}

	// Test 4: Technical query -> Logic & Curiosity increase
	p4, c4 := EvolvePersonalityVector(initial, TurnReflection{}, "帮我写段代码，为什么这行会报错？底层原理是什么？")
	if !c4 || p4.Logic != 51 || p4.Curiosity != 71 {
		t.Errorf("expected logic & curiosity +1, got %+v (changed=%v)", p4, c4)
	}

	// Test 4b: write-code keyword alone
	p4b, c4b := EvolvePersonalityVector(initial, TurnReflection{}, "帮我写代码")
	if !c4b || p4b.Logic != 51 || p4b.Curiosity != 71 {
		t.Errorf("expected write-code to bump logic & curiosity, got %+v (changed=%v)", p4b, c4b)
	}

	// Test 5: User scolds/abuses -> Warmth decreases, Sarcasm increases
	p5, c5 := EvolvePersonalityVector(initial, TurnReflection{}, "笨蛋，闭嘴，别烦我！")
	if !c5 || p5.Warmth != 68 || p5.Sarcasm != 12 {
		t.Errorf("expected warmth -2, sarcasm +2, got %+v (changed=%v)", p5, c5)
	}

	// Test 5b: dislike keyword
	p5b, c5b := EvolvePersonalityVector(initial, TurnReflection{}, "讨厌你")
	if !c5b || p5b.Warmth != 68 || p5b.Sarcasm != 12 {
		t.Errorf("expected 讨厌 to decrease warmth and increase sarcasm, got %+v (changed=%v)", p5b, c5b)
	}

	// Test 6: Empathy worked -> Empathy & Warmth increase
	p6, c6 := EvolvePersonalityVector(initial, TurnReflection{EmpathyWorked: true}, "")
	if !c6 || p6.Empathy != 81 || p6.Warmth != 71 {
		t.Errorf("expected empathy & warmth +1, got %+v (changed=%v)", p6, c6)
	}

	// Test 7: clamp upper bound
	maxed := initial
	maxed.Humor = 100
	maxed.Confidence = 100
	p7, c7 := EvolvePersonalityVector(maxed, TurnReflection{}, "哈哈真有趣")
	if !c7 || p7.Humor != 100 || p7.Confidence != 100 {
		t.Errorf("expected clamp at 100, got %+v", p7)
	}

	// Test 8: clamp lower bound
	low := initial
	low.Energy = 1
	low.Warmth = 0
	p8, c8 := EvolvePersonalityVector(low, TurnReflection{StyleNote: "太长啰嗦小作文"}, "")
	if !c8 || p8.Energy != 0 || p8.Warmth != 0 {
		t.Errorf("expected clamp at 0, got energy=%d warmth=%d", p8.Energy, p8.Warmth)
	}
}
