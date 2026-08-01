package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/text"
	"github.com/mochi-ai/server/pkg/dashscope"
)

func TestProsodyForMood(t *testing.T) {
	baseline := VoiceProfile{Rate: "+0%", Pitch: "+0Hz"}
	gentle := ProsodyForMood(text.MoodGentle, baseline)
	if gentle.Rate >= 1.0 || gentle.Pitch >= 1.0 {
		t.Errorf("gentle should slow/down pitch: %+v", gentle)
	}
	if gentle.Rate > 0.86 {
		t.Errorf("gentle rate should be ~0.85, got %v", gentle.Rate)
	}
	excited := ProsodyForMood(text.MoodExcited, baseline)
	if excited.Rate <= 1.0 || excited.Pitch <= 1.0 {
		t.Errorf("excited should speed/up pitch: %+v", excited)
	}
	if excited.Rate < 1.10 {
		t.Errorf("excited rate should be noticeably faster, got %v", excited.Rate)
	}
	if gentle.Rate >= excited.Rate {
		t.Error("gentle and excited should be audibly different")
	}
}

func TestParseVoiceBaseline(t *testing.T) {
	r, p := parseVoiceBaseline(VoiceProfile{Rate: "+5%", Pitch: "-3%"})
	if r < 1.04 || r > 1.06 {
		t.Errorf("rate baseline = %v", r)
	}
	if p < 0.96 || p > 0.98 {
		t.Errorf("pitch baseline = %v", p)
	}
}

func TestProsodyToSynthOptions(t *testing.T) {
	opts := ProsodyParams{Rate: 0.92, Pitch: 0.95, Volume: 48}.ToSynthOptions()
	if opts != (dashscope.SynthOptions{Rate: 0.92, Pitch: 0.95, Volume: 48}) {
		t.Errorf("unexpected synth options: %+v", opts)
	}
}
