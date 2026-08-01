package realtime

import (
	"testing"

	"github.com/mochi-ai/server/internal/text"
)

// extractProsodyTag 解析分句 mood 标记并映射 TTS 参数（供 pipeline 与测试共用）。
func extractProsodyTag(raw string, baseline VoiceProfile, tracker *text.MoodTracker) (cleanText string, params ProsodyParams) {
	if tracker == nil {
		tracker = text.NewMoodTracker()
	}
	seg := tracker.Process(raw)
	opts := ProsodyForMood(seg.Mood, baseline)
	return seg.Text, opts
}

func TestExtractProsodyTag(t *testing.T) {
	baseline := VoiceProfile{Rate: "+0%", Pitch: "+0Hz"}
	tracker := text.NewMoodTracker()
	textOut, params := extractProsodyTag("[mood:gentle] 你好呀！", baseline, tracker)
	if textOut != "你好呀！" {
		t.Errorf("text = %q", textOut)
	}
	if params.Rate >= 1.0 {
		t.Errorf("gentle rate should be < 1.0, got %v", params.Rate)
	}
}
