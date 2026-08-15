package learning

import (
	"context"
	"testing"
)

func TestDetectLearningIntent(t *testing.T) {
	tests := []struct {
		input       string
		wantMode    LearningMode
		wantScene   string
		wantMatched bool
	}{
		{
			input:       "帮我带娃，给宝宝讲个绘本故事",
			wantMode:    ModeKidsStory,
			wantScene:   "picture_book",
			wantMatched: true,
		},
		{
			input:       "我想练一下英语面试，模拟外企面试官",
			wantMode:    ModeAdultRoleplay,
			wantScene:   "job_interview",
			wantMatched: true,
		},
		{
			input:       "跟我用英文聊聊天吧，想练习一下口语",
			wantMode:    ModeAdultFreeTalk,
			wantScene:   "free_talk",
			wantMatched: true,
		},
		{
			input:       "Hello Mochi, how are you doing today?",
			wantMode:    ModeAdultFreeTalk,
			wantScene:   "auto_english",
			wantMatched: true,
		},
		{
			input:       "今天深圳的天气怎么样呀",
			wantMode:    ModeCompanion,
			wantScene:   "",
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		gotMode, gotScene, gotMatched := DetectLearningIntent(tt.input)
		if gotMatched != tt.wantMatched {
			t.Errorf("DetectLearningIntent(%q) matched = %v, want %v", tt.input, gotMatched, tt.wantMatched)
		}
		if gotMode != tt.wantMode {
			t.Errorf("DetectLearningIntent(%q) mode = %v, want %v", tt.input, gotMode, tt.wantMode)
		}
		if gotScene != tt.wantScene {
			t.Errorf("DetectLearningIntent(%q) scene = %v, want %v", tt.input, gotScene, tt.wantScene)
		}
	}
}

func TestLearningServiceProfile(t *testing.T) {
	svc := NewService(nil)
	profile := svc.GetProfile(context.Background(), 1)
	if profile.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", profile.UserID)
	}

	profile.Mode = ModeAdultRoleplay
	profile.TargetScene = "job_interview"
	svc.SetProfile(context.Background(), profile)

	updated := svc.GetProfile(context.Background(), 1)
	if updated.Mode != ModeAdultRoleplay || updated.TargetScene != "job_interview" {
		t.Errorf("expected updated profile, got %+v", updated)
	}
}
