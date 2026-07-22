package realtime

import (
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func TestTakeFlushSegment_ChinesePunctuation(t *testing.T) {
	cfg := config.RealtimePipeline{TTSMinChars: 5, TTSPunctuation: "。！？"}
	var buf strings.Builder
	buf.WriteString("你好呀，我是 Mochi。")
	seg := takeFlushSegment(&buf, cfg)
	if seg != "你好呀，我是 Mochi。" {
		t.Fatalf("got %q", seg)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty remainder, got %q", buf.String())
	}
}

func TestTakeFlushSegment_PartialBuffer(t *testing.T) {
	cfg := config.RealtimePipeline{TTSMinChars: 5, TTSPunctuation: "。！？"}
	var buf strings.Builder
	buf.WriteString("你好")
	if seg := takeFlushSegment(&buf, cfg); seg != "" {
		t.Fatalf("expected no flush, got %q", seg)
	}
	buf.WriteString("，世界！")
	seg := takeFlushSegment(&buf, cfg)
	if seg != "你好，世界！" {
		t.Fatalf("got %q", seg)
	}
}

func TestTakeFlushSegment_LongNoPunctuation(t *testing.T) {
	cfg := config.RealtimePipeline{TTSMinChars: 3, TTSPunctuation: "。！？"}
	var buf strings.Builder
	buf.WriteString("abcdefghijkl")
	seg := takeFlushSegment(&buf, cfg)
	if seg != "abcdefghijkl" {
		t.Fatalf("got %q", seg)
	}
}
