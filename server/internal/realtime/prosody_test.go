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
	excited := ProsodyForMood(text.MoodExcited, baseline)
	if excited.Rate <= 1.0 || excited.Pitch <= 1.0 {
		t.Errorf("excited should speed/up pitch: %+v", excited)
	}
	if gentle.Rate < 0.85 || excited.Rate > 1.15 {
		t.Error("rate out of clamp range")
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
