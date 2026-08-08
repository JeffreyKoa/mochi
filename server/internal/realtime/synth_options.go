package realtime

// SynthOptions 控制 TTS 合成的语速/音高/音量（供 x-tts 等引擎使用）。
type SynthOptions struct {
	Rate   float64
	Pitch  float64
	Volume int
}

// DefaultSynthOptions 返回默认合成参数。
func DefaultSynthOptions() SynthOptions {
	return SynthOptions{Rate: 1.0, Pitch: 1.0, Volume: 50}
}
