package realtime

import (
	"strconv"
	"strings"

	"github.com/mochi-ai/server/internal/text"
	"github.com/mochi-ai/server/pkg/dashscope"
)

// ProsodyParams 为 TTS 合成用的语速/音高/音量。
type ProsodyParams struct {
	Rate   float64
	Pitch  float64
	Volume int
}

// ToSynthOptions 转为 DashScope TTS 参数。
func (p ProsodyParams) ToSynthOptions() dashscope.SynthOptions {
	return dashscope.SynthOptions{
		Rate:   p.Rate,
		Pitch:  p.Pitch,
		Volume: p.Volume,
	}
}

// ProsodyForMood 基于 session 音色基线与 mood 计算逐句 prosody。
func ProsodyForMood(mood text.MoodTag, baseline VoiceProfile) ProsodyParams {
	rate, pitch := parseVoiceBaseline(baseline)
	vol := 50

	// Phase 4：加大 mood 间 prosody 差异，使安慰 vs 报喜可听区分更明显。
	switch mood {
	case text.MoodGentle:
		rate *= 0.85
		pitch *= 0.90
		vol = 42
	case text.MoodExcited:
		rate *= 1.12
		pitch *= 1.10
		vol = 58
	case text.MoodSad:
		rate *= 0.82
		pitch *= 0.88
		vol = 40
	case text.MoodWorried:
		rate *= 0.88
		pitch *= 0.92
		vol = 43
	case text.MoodPlayful:
		rate *= 1.10
		pitch *= 1.08
		vol = 54
	case text.MoodSerious:
		rate *= 0.94
		pitch *= 0.96
		vol = 48
	default: // calm
		// 使用基线
	}

	return ProsodyParams{
		Rate:   clamp(rate, 0.85, 1.15),
		Pitch:  clamp(pitch, 0.90, 1.10),
		Volume: clampInt(vol, 40, 60),
	}
}

// parseVoiceBaseline 将 VoiceProfile 的 Rate/Pitch 字符串解析为乘数。
func parseVoiceBaseline(v VoiceProfile) (rate, pitch float64) {
	rate = parsePercentMultiplier(v.Rate, 1.0)
	pitch = parsePercentMultiplier(v.Pitch, 1.0)
	return rate, pitch
}

func parsePercentMultiplier(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "+0%" || raw == "+0Hz" {
		return fallback
	}
	sign := 1.0
	if strings.HasPrefix(raw, "-") {
		sign = -1.0
	}
	numStr := strings.TrimPrefix(strings.TrimPrefix(raw, "+"), "-")
	numStr = strings.TrimSuffix(strings.TrimSuffix(numStr, "%"), "Hz")
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return fallback
	}
	return 1.0 + sign*val/100.0
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
