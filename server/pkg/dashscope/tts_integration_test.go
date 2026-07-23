package dashscope

import (
	"context"
	"os"
	"testing"
	"time"
)

// Manual: DASHSCOPE_API_KEY=sk-... go test -run TestTTS -v ./pkg/dashscope/

func TestTTSCosyVoiceV2(t *testing.T) {
	key := os.Getenv("DASHSCOPE_API_KEY")
	if key == "" {
		t.Skip("DASHSCOPE_API_KEY not set")
	}
	client := NewTTSClient(key, "cosyvoice-v2", "longxiaochun_v2", 22050)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var n int
	err := client.Synthesize(ctx, "你好，我是 Mochi。", func(b []byte) {
		n += len(b)
	})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if n == 0 {
		t.Fatal("no audio received")
	}
	t.Logf("ok, %d bytes audio", n)
}

func TestTTSQwenAudio(t *testing.T) {
	key := os.Getenv("DASHSCOPE_API_KEY")
	if key == "" {
		t.Skip("DASHSCOPE_API_KEY not set")
	}
	client := NewTTSClient(key, "qwen-audio-3.0-tts-plus", "longanhuan_v3.6", 22050)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var n int
	err := client.Synthesize(ctx, "你好，我是 Mochi。", func(b []byte) {
		n += len(b)
	})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if n == 0 {
		t.Fatal("no audio received")
	}
	t.Logf("ok, %d bytes audio", n)
}
