package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/text"
)

func TestProsodyForMood(t *testing.T) {
	baseline := VoiceProfile{Rate: "+0%", Pitch: "+0Hz"}
	p := ProsodyForMood(text.MoodGentle, baseline)
	if p.Rate >= 1.0 {
		t.Errorf("gentle rate should be slower, got %v", p.Rate)
	}
}

func TestParseVoiceBaseline(t *testing.T) {
	r, p := parseVoiceBaseline(VoiceProfile{Rate: "+5%", Pitch: "-3%"})
	if r < 1.0 || r > 1.1 {
		t.Errorf("rate = %v", r)
	}
	if p > 1.0 || p < 0.9 {
		t.Errorf("pitch = %v", p)
	}
}

func TestProsodyParams_ToSynthOptions(t *testing.T) {
	opts := ProsodyParams{Rate: 0.92, Pitch: 0.95, Volume: 48}.ToSynthOptions()
	if opts != (SynthOptions{Rate: 0.92, Pitch: 0.95, Volume: 48}) {
		t.Fatalf("unexpected opts: %+v", opts)
	}
}
