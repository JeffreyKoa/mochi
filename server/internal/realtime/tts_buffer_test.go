package realtime

import (
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func testPipelineCfg() config.RealtimePipeline {
	p := config.RealtimePipeline{
		TTSMinChars:          4,
		TTSFirstMinChars:     3,
		TTSWeakPunctMinChars: 8,
		TTSForceFlushChars:   24,
		TTSPunctuation:       "。！？，",
		TTSStrongPunctuation: "。！？",
		TTSWeakPunctuation:   "，",
	}
	return p
}

func testNoisePipeline() *Pipeline {
	return &Pipeline{noiseFillers: config.BuildNoiseFillerSet([]string{
		"啊", "呃", "哦", "哼", "咳",
	})}
}

func TestTakeFlushSegment_ChinesePunctuation(t *testing.T) {
	cfg := testPipelineCfg()
	var buf strings.Builder
	buf.WriteString("你好呀，我是 Mochi。")
	seg := takeFlushSegment(&buf, cfg)
	if seg != "你好呀，" {
		t.Fatalf("first segment got %q", seg)
	}
	seg2 := takeFlushSegmentEx(&buf, cfg, false)
	if seg2 != "我是 Mochi。" {
		t.Fatalf("second segment got %q", seg2)
	}
}

func TestTakeFlushSegment_MultiSentenceForward(t *testing.T) {
	cfg := testPipelineCfg()
	var buf strings.Builder
	buf.WriteString("你好呀！我是 Mochi。今天天气真是不错呢！")

	seg1 := takeFlushSegmentEx(&buf, cfg, true)
	if seg1 != "你好呀！" {
		t.Fatalf("seg1 got %q, expected %q", seg1, "你好呀！")
	}

	seg2 := takeFlushSegmentEx(&buf, cfg, false)
	if seg2 != "我是 Mochi。" {
		t.Fatalf("seg2 got %q, expected %q", seg2, "我是 Mochi。")
	}

	seg3 := takeFlushSegmentEx(&buf, cfg, false)
	if seg3 != "今天天气真是不错呢！" {
		t.Fatalf("seg3 got %q, expected %q", seg3, "今天天气真是不错呢！")
	}
}

func TestIsNoiseTranscript(t *testing.T) {
	p := testNoisePipeline()
	if !p.isNoiseTranscript("咳") {
		t.Fatalf("expected noise for '咳'")
	}
	if !p.isNoiseTranscript("呃 哼") {
		t.Fatalf("expected noise for '呃 哼'")
	}
	if p.isNoiseTranscript("好") {
		t.Fatalf("expected valid for '好'")
	}
	if p.isNoiseTranscript("对") {
		t.Fatalf("expected valid for '对'")
	}
	if p.isNoiseTranscript("行") {
		t.Fatalf("expected valid for '行'")
	}
	if p.isNoiseTranscript("嗯") {
		t.Fatalf("expected valid for '嗯'")
	}
	if p.isNoiseTranscript("喂") {
		t.Fatalf("expected valid for '喂'")
	}
}
