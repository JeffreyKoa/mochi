package realtime

import (
	"log"
	"sync"
	"time"
)

// TurnLatency records per-turn pipeline timestamps relative to turn origin.
type TurnLatency struct {
	mu sync.Mutex

	origin time.Time

	audioEnd         time.Time
	asrFinal         time.Time
	llmFirstToken    time.Time
	llmFirstSentence time.Time
	ttsFirstByte     time.Time
	playbackStart    time.Time
}

func NewTurnLatency(origin time.Time) *TurnLatency {
	if origin.IsZero() {
		origin = time.Now()
	}
	return &TurnLatency{origin: origin}
}

func (t *TurnLatency) Origin() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.origin
}

func (t *TurnLatency) MarkAudioEnd() {
	t.mark(&t.audioEnd)
}

func (t *TurnLatency) MarkASRFinal() {
	t.mark(&t.asrFinal)
}

func (t *TurnLatency) MarkLLMFirstToken() {
	t.mark(&t.llmFirstToken)
}

func (t *TurnLatency) MarkLLMFirstSentence() {
	t.mark(&t.llmFirstSentence)
}

func (t *TurnLatency) MarkTTSFirstByte() {
	t.mark(&t.ttsFirstByte)
}

func (t *TurnLatency) MarkPlaybackStart() {
	t.mark(&t.playbackStart)
}

// MarkPlaybackFromClient records client-side playback start relative to turn origin.
func (t *TurnLatency) MarkPlaybackFromClient(atMS int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if atMS < 0 {
		return
	}
	if t.playbackStart.IsZero() {
		t.playbackStart = t.origin.Add(time.Duration(atMS) * time.Millisecond)
	}
}

func (t *TurnLatency) mark(dst *time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if dst.IsZero() {
		*dst = time.Now()
	}
}

func (t *TurnLatency) sinceOrigin(at time.Time) int64 {
	if at.IsZero() {
		return -1
	}
	return at.Sub(t.origin).Milliseconds()
}

func (t *TurnLatency) ToMetrics() TurnMetrics {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TurnMetrics{
		AudioEndMS:         t.sinceOrigin(t.audioEnd),
		ASRFinalMS:         t.sinceOrigin(t.asrFinal),
		LLMFirstTokenMS:    t.sinceOrigin(t.llmFirstToken),
		LLMFirstSentenceMS: t.sinceOrigin(t.llmFirstSentence),
		TTSFirstByteMS:     t.sinceOrigin(t.ttsFirstByte),
		PlaybackStartMS:    t.sinceOrigin(t.playbackStart),
	}
}

func (t *TurnLatency) LogSummary(sessionID string) {
	m := t.ToMetrics()
	log.Printf(
		"[realtime] latency session=%s audio_end=%dms asr=%dms llm_ttft=%dms llm_sentence=%dms tts_ttfb=%dms playback=%dms",
		sessionID,
		m.AudioEndMS,
		m.ASRFinalMS,
		m.LLMFirstTokenMS,
		m.LLMFirstSentenceMS,
		m.TTSFirstByteMS,
		m.PlaybackStartMS,
	)
}
