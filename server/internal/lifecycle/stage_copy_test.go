package lifecycle

import (
	"strings"
	"testing"
)

func TestStagePromptSection_singleBlock(t *testing.T) {
	got := StagePromptSection("newborn", "cat")
	if strings.Contains(got, "\n\n") {
		t.Fatalf("expected single paragraph, got multiline block: %q", got)
	}
	for _, want := range []string{"你刚出生不久", "禁止：宠物式拟人表演"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestStagePromptSection_tiger(t *testing.T) {
	got := StagePromptSection("youth", "tiger")
	if !strings.Contains(got, "幻想伙伴") {
		t.Fatalf("expected tiger prompt suffix, got %q", got)
	}
}

func TestPromptFragment_matchesStagePromptSection(t *testing.T) {
	if got, want := PromptFragment("prime", "dog_small"), StagePromptSection("prime", "dog_small"); got != want {
		t.Fatalf("PromptFragment drift: got %q want %q", got, want)
	}
}

func TestEffectiveSpeechStyle_override(t *testing.T) {
	const custom = "主人指定的风格"
	got := EffectiveSpeechStyle("newborn", "cat", custom)
	if got != custom {
		t.Fatalf("got %q, want override %q", got, custom)
	}
}

func TestStageHintForUser_groupedStages(t *testing.T) {
	juvenile := StageHintForUser("juvenile")
	child := StageHintForUser("child")
	if juvenile != child {
		t.Fatalf("juvenile and child hints should match: %q vs %q", juvenile, child)
	}
}
