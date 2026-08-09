package sidecarlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeReqBodyRedactsPCM(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"pcm_base64":  strings.Repeat("A", 100),
		"sample_rate": 16000,
	})
	got := summarizeReqBody(body)
	if strings.Contains(got, strings.Repeat("A", 20)) {
		t.Fatalf("expected redacted pcm_base64, got %q", got)
	}
	if !strings.Contains(got, "len=100") {
		t.Fatalf("expected len hint, got %q", got)
	}
}

func TestSummarizeHTTPRespBinary(t *testing.T) {
	got := summarizeHTTPResp(200, make([]byte, 8192))
	if !strings.Contains(got, "body_bytes=8192") {
		t.Fatalf("got %q", got)
	}
}

func TestWSBatchChunkBytes(t *testing.T) {
	if WSBatchChunkBytes() != 640 {
		t.Fatalf("chunk=%d want 640", WSBatchChunkBytes())
	}
}
