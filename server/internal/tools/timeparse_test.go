package tools

import (
	"testing"
	"time"
)

func TestParseFireAtTomorrow(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, loc)
	fire, ok := ParseFireAt("明天早上9点提醒我开会", now)
	if !ok {
		t.Fatal("expected ok")
	}
	if fire.Day() != 24 || fire.Hour() != 9 || fire.Minute() != 0 {
		t.Fatalf("unexpected fire time: %v", fire)
	}
}

func TestParseFireAtChineseNumerals(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, loc)
	cases := []struct {
		msg      string
		wantDay  int
		wantHour int
		wantMin  int
	}{
		{"明天早上九点钟开会，帮我记一下。", 24, 9, 0},
		{"明天早上九点半开会，帮我记一下，提醒我。", 24, 9, 30},
		{"明天早上十点钟开会，帮我记一下，到时候提醒我。", 24, 10, 0},
	}
	for _, c := range cases {
		fire, ok := ParseFireAt(c.msg, now)
		if !ok {
			t.Fatalf("parse failed: %q", c.msg)
		}
		if fire.Day() != c.wantDay || fire.Hour() != c.wantHour || fire.Minute() != c.wantMin {
			t.Fatalf("%q => %v want day=%d hour=%d min=%d", c.msg, fire, c.wantDay, c.wantHour, c.wantMin)
		}
	}

	nowLate := time.Date(2026, 7, 23, 21, 0, 0, 0, loc)
	fire, ok := ParseFireAt("今天晚上11点半，我要上个洗手间。", nowLate)
	if !ok {
		t.Fatal("parse failed tonight")
	}
	if fire.Day() != 23 || fire.Hour() != 23 || fire.Minute() != 30 {
		t.Fatalf("tonight => %v", fire)
	}
}

func TestParseTimeISO(t *testing.T) {
	tm, err := parseTimeISO("2026-07-24T09:00:00+08:00", "")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Hour() != 9 {
		t.Fatalf("hour=%d", tm.Hour())
	}
}

func TestExtractTodoTitle(t *testing.T) {
	title := ExtractTodoTitle("帮我把买牛奶记下来")
	if title != "买牛奶" {
		t.Fatalf("expected 买牛奶, got %q", title)
	}
}

func TestRegistryCount(t *testing.T) {
	if len(Registry()) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(Registry()))
	}
}
