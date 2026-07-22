package realtime

import (
	"testing"
	"time"
)

func TestTurnLatency_Marks(t *testing.T) {
	origin := time.Now()
	lat := NewTurnLatency(origin)

	lat.MarkAudioEnd()
	lat.MarkASRFinal()
	lat.MarkLLMFirstToken()
	lat.MarkLLMFirstSentence()
	lat.MarkTTSFirstByte()
	lat.MarkPlaybackFromClient(1200)

	m := lat.ToMetrics()
	if m.AudioEndMS < 0 || m.ASRFinalMS < 0 || m.LLMFirstTokenMS < 0 {
		t.Fatalf("expected non-negative server marks, got %+v", m)
	}
	if m.PlaybackStartMS != 1200 {
		t.Fatalf("playback start ms = %d, want 1200", m.PlaybackStartMS)
	}
}

func TestTurnLatency_SinceOriginUnset(t *testing.T) {
	lat := NewTurnLatency(time.Now())
	m := lat.ToMetrics()
	if m.TTSFirstByteMS != -1 {
		t.Fatalf("unset mark should be -1, got %d", m.TTSFirstByteMS)
	}
}
