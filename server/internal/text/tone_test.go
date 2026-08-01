package text

import "testing"

func TestParseToneSegment(t *testing.T) {
	tests := []struct {
		in       string
		wantMood MoodTag
		wantText string
	}{
		{"[mood:gentle]没事的。", MoodGentle, "没事的。"},
		{"  [mood:excited]太好了！", MoodExcited, "太好了！"},
		{"你好呀", "", "你好呀"},
		{"[mood:unknown]嗨", "", "[mood:unknown]嗨"},
	}
	for _, tt := range tests {
		got := ParseToneSegment(tt.in)
		if got.Mood != tt.wantMood || got.Text != tt.wantText {
			t.Errorf("ParseToneSegment(%q) = {%q,%q}, want {%q,%q}", tt.in, got.Mood, got.Text, tt.wantMood, tt.wantText)
		}
	}
}

func TestStripMoodTags(t *testing.T) {
	in := "[mood:gentle]没事。[mood:excited]太好了！"
	want := "没事。太好了！"
	if got := StripMoodTags(in); got != want {
		t.Errorf("StripMoodTags = %q, want %q", got, want)
	}
}

func TestMoodTracker_Inherit(t *testing.T) {
	mt := NewMoodTracker()
	s1 := mt.Process("[mood:gentle]第一句。")
	if s1.Mood != MoodGentle || s1.Text != "第一句。" {
		t.Fatalf("first segment: %+v", s1)
	}
	s2 := mt.Process("第二句。")
	if s2.Mood != MoodGentle || s2.Text != "第二句。" {
		t.Fatalf("inherited segment: %+v", s2)
	}
}

func TestStreamMoodStripper(t *testing.T) {
	var sm StreamMoodStripper
	out := sm.Feed("[mood:gentle]")
	if out != "" {
		t.Errorf("partial tag should emit nothing, got %q", out)
	}
	out = sm.Feed("你好。")
	if out != "你好。" {
		t.Errorf("after tag closed, got %q", out)
	}
}
