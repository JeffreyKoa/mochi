package realtime

import (
	"testing"
)

func TestResolveVoice(t *testing.T) {
	tests := []struct {
		name        string
		gender      string
		lifeStage   string
		personality string
		wantDash    string
		wantEdge    string
		wantRate    string
		wantPitch   string
	}{
		{
			name:        "Female Newborn",
			gender:      "female",
			lifeStage:   "newborn",
			personality: "",
			wantDash:    "longxiaochun_v2",
			wantEdge:    "zh-CN-XiaoyiNeural",
			wantRate:    "+0%",
			wantPitch:   "+0Hz",
		},
		{
			name:        "Female Youth",
			gender:      "female",
			lifeStage:   "youth",
			personality: "",
			wantDash:    "longwan_v2",
			wantEdge:    "zh-CN-XiaoxiaoNeural",
			wantRate:    "+0%",
			wantPitch:   "+0Hz",
		},
		{
			name:        "Female Elder",
			gender:      "female",
			lifeStage:   "elder",
			personality: "",
			wantDash:    "longwan_v2",
			wantEdge:    "zh-CN-XiaohanNeural",
			wantRate:    "-8%",
			wantPitch:   "+0Hz",
		},
		{
			name:        "Male Youth",
			gender:      "male",
			lifeStage:   "youth",
			personality: "",
			wantDash:    "longyue_v2",
			wantEdge:    "zh-CN-YunjianNeural",
			wantRate:    "+0%",
			wantPitch:   "+0Hz",
		},
		{
			name:        "Male Prime",
			gender:      "male",
			lifeStage:   "prime",
			personality: "",
			wantDash:    "longshu_v2",
			wantEdge:    "zh-CN-YunxiNeural",
			wantRate:    "+0%",
			wantPitch:   "+0Hz",
		},
		{
			name:        "Male Elder",
			gender:      "male",
			lifeStage:   "elder",
			personality: "",
			wantDash:    "longshu_v2",
			wantEdge:    "zh-CN-YunxiNeural",
			wantRate:    "-10%",
			wantPitch:   "-5%",
		},
		{
			name:        "Male Youth Energetic Personality",
			gender:      "male",
			lifeStage:   "youth",
			personality: "阳光",
			wantDash:    "longyue_v2",
			wantEdge:    "zh-CN-YunjianNeural",
			wantRate:    "+5%",
			wantPitch:   "+0Hz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveVoice(tt.gender, tt.lifeStage, tt.personality)
			if got.DashscopeVoice != tt.wantDash {
				t.Errorf("DashscopeVoice = %v, want %v", got.DashscopeVoice, tt.wantDash)
			}
			if got.EdgeVoice != tt.wantEdge {
				t.Errorf("EdgeVoice = %v, want %v", got.EdgeVoice, tt.wantEdge)
			}
			if got.Rate != tt.wantRate {
				t.Errorf("Rate = %v, want %v", got.Rate, tt.wantRate)
			}
			if got.Pitch != tt.wantPitch {
				t.Errorf("Pitch = %v, want %v", got.Pitch, tt.wantPitch)
			}
		})
	}
}
