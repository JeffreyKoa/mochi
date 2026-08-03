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
	// 带空格的 mood 标记
	spaced := "[mood: calm]今晚深圳25°C。"
	if got := StripMoodTags(spaced); got != "今晚深圳25°C。" {
		t.Errorf("spaced mood tag = %q", got)
	}
	// 残缺标记
	orphan := "m]今晚。[mood:playful]你穿"
	if got := StripMoodTags(orphan); got != "今晚。你穿" {
		t.Errorf("orphan cleanup = %q, want 今晚。你穿", got)
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

func TestNewMoodTrackerWithDefault(t *testing.T) {
	mt := NewMoodTrackerWithDefault(MoodGentle)
	s := mt.Process("没有标记的一句。")
	if s.Mood != MoodGentle {
		t.Fatalf("expected gentle default, got %q", s.Mood)
	}
}

func TestInferDefaultMood(t *testing.T) {
	if got := InferDefaultMood("sad", "chat", false); got != MoodGentle {
		t.Errorf("sad -> gentle, got %q", got)
	}
	if got := InferDefaultMood("happy", "joke", false); got != MoodPlayful {
		t.Errorf("happy joke -> playful, got %q", got)
	}
	if got := InferDefaultMood("neutral", "chat", false); got != MoodCalm {
		t.Errorf("neutral -> calm, got %q", got)
	}
}

func TestMoodTagComplianceRate(t *testing.T) {
	full := "[mood:gentle]没事。[mood:calm]我在呢。"
	if rate := MoodTagComplianceRate(full); rate < 0.99 {
		t.Errorf("full compliance expected, got %v", rate)
	}
	partial := "[mood:gentle]没事。第二句没标。"
	if rate := MoodTagComplianceRate(partial); rate < 0.49 || rate > 0.51 {
		t.Errorf("half compliance expected ~0.5, got %v", rate)
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
	// 流式切在 mood 中间
	sm = StreamMoodStripper{}
	if sm.Feed("[mood:cal") != "" {
		t.Error("mid-tag chunk 1 should be silent")
	}
	if got := sm.Feed("m]今晚"); got != "今晚" {
		t.Errorf("mid-tag chunk 2 = %q, want 今晚", got)
	}
}
