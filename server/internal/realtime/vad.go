package realtime

import (
	"encoding/binary"
	"math"
)

// EnergyVAD simple RMS-based voice activity detection for Phase 1.
// Phase 2 will replace with Silero VAD.
type EnergyVAD struct {
	SilenceMS   int
	MinSpeechMS int
	SampleRate  int

	speechActive   bool
	speechStartMS  int64
	lastSpeechMS   int64
	silenceSinceMS int64
	totalMS        int64
}

func NewEnergyVAD(sampleRate, silenceMS, minSpeechMS int) *EnergyVAD {
	if sampleRate == 0 {
		sampleRate = 16000
	}
	if silenceMS == 0 {
		silenceMS = 800
	}
	if minSpeechMS == 0 {
		minSpeechMS = 300
	}
	return &EnergyVAD{
		SampleRate:  sampleRate,
		SilenceMS:   silenceMS,
		MinSpeechMS: minSpeechMS,
	}
}

// Feed processes PCM int16 LE chunk. Returns speech_start / speech_end / "".
func (v *EnergyVAD) Feed(pcm []byte) string {
	if len(pcm) < 2 {
		return ""
	}

	samples := len(pcm) / 2
	chunkMS := int64(samples) * 1000 / int64(v.SampleRate)
	v.totalMS += chunkMS

	rms := pcmRMS(pcm)
	const threshold = 60.0

	if rms >= threshold {
		v.lastSpeechMS = v.totalMS
		v.silenceSinceMS = 0
		if !v.speechActive {
			v.speechActive = true
			v.speechStartMS = v.totalMS
			return "speech_start"
		}
		return ""
	}

	if !v.speechActive {
		return ""
	}

	if v.silenceSinceMS == 0 {
		v.silenceSinceMS = v.totalMS
	}

	silenceDuration := v.totalMS - v.lastSpeechMS
	speechDuration := v.lastSpeechMS - v.speechStartMS

	if silenceDuration >= int64(v.SilenceMS) && speechDuration >= int64(v.MinSpeechMS) {
		v.speechActive = false
		v.silenceSinceMS = 0
		return "speech_end"
	}
	return ""
}

func (v *EnergyVAD) Reset() {
	v.speechActive = false
	v.speechStartMS = 0
	v.lastSpeechMS = 0
	v.silenceSinceMS = 0
}

func pcmRMS(pcm []byte) float64 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(n))
}
