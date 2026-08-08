package realtime

import (
	"strings"
)

// VoiceProfile 按性别/生命阶段/性格解析 TTS 语速与音高基线（本地 Matcha 通过 prosody 调节）。
type VoiceProfile struct {
	Rate  string
	Pitch string
}

// ResolveVoice returns the voice profile determined by gender, lifeStage, and personality.
func ResolveVoice(gender, lifeStage, personality string) VoiceProfile {
	g := strings.ToLower(strings.TrimSpace(gender))
	stage := strings.ToLower(strings.TrimSpace(lifeStage))
	p := strings.ToLower(strings.TrimSpace(personality))

	if g != "male" {
		g = "female"
	}

	profile := VoiceProfile{
		Rate:  "+0%",
		Pitch: "+0Hz",
	}

	if g == "male" {
		switch stage {
		case "elder", "twilight":
			profile.Rate = "-10%"
			profile.Pitch = "-5%"
		}
	} else {
		switch stage {
		case "elder", "twilight":
			profile.Rate = "-8%"
		}
	}

	// 性格微调（老年阶段已设语速时不再覆盖）
	if profile.Rate == "+0%" {
		if strings.Contains(p, "阳光") || strings.Contains(p, "活泼") || strings.Contains(p, "energetic") {
			profile.Rate = "+5%"
		} else if strings.Contains(p, "沉稳") || strings.Contains(p, "知性") || strings.Contains(p, "gentle") {
			profile.Rate = "-3%"
		}
	}

	return profile
}
