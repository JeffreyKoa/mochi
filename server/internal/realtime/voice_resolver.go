package realtime

import (
	"strings"
)

// VoiceProfile holds the resolved TTS voice parameters based on gender, life stage, and personality.
type VoiceProfile struct {
	DashscopeVoice string
	Rate           string
	Pitch          string
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
		case "newborn", "child", "juvenile":
			profile.DashscopeVoice = "longyue_v2"
		case "youth":
			profile.DashscopeVoice = "longyue_v2"
		case "prime":
			profile.DashscopeVoice = "longshu_v2"
		case "elder", "twilight":
			profile.DashscopeVoice = "longshu_v2"
			profile.Rate = "-10%"
			profile.Pitch = "-5%"
		default: // default male youth/prime
			profile.DashscopeVoice = "longyue_v2"
		}
	} else { // female
		switch stage {
		case "newborn", "child", "juvenile":
			profile.DashscopeVoice = "longxiaochun_v2"
		case "youth", "prime":
			profile.DashscopeVoice = "longwan_v2"
		case "elder", "twilight":
			profile.DashscopeVoice = "longwan_v2"
			profile.Rate = "-8%"
		default: // default female
			profile.DashscopeVoice = "longxiaochun_v2"
		}
	}

	// Personality fine-tuning (if not overridden by elder/twilight rates)
	if profile.Rate == "+0%" {
		if strings.Contains(p, "阳光") || strings.Contains(p, "活泼") || strings.Contains(p, "energetic") {
			profile.Rate = "+5%"
		} else if strings.Contains(p, "沉稳") || strings.Contains(p, "知性") || strings.Contains(p, "gentle") {
			profile.Rate = "-3%"
		}
	}

	return profile
}
