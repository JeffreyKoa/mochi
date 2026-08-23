package realtime

import "testing"

func TestSessionBumpTTSEpoch(t *testing.T) {
	s := &Session{ID: "test"}
	if s.TTSEpoch() != 0 {
		t.Fatalf("initial epoch want 0 got %d", s.TTSEpoch())
	}
	e1 := s.BumpTTSEpoch()
	e2 := s.BumpTTSEpoch()
	if e1 != 1 || e2 != 2 {
		t.Fatalf("epoch sequence got %d %d", e1, e2)
	}
}

func TestCancelPipelineBumpsEpoch(t *testing.T) {
	s := &Session{ID: "test"}
	before := s.TTSEpoch()
	after := s.CancelPipeline()
	if after != before+1 {
		t.Fatalf("cancel bump got before=%d after=%d", before, after)
	}
}
