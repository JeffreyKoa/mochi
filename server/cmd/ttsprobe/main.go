package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mochi-ai/server/pkg/dashscope"
)

func try(key, model, voice string, ep dashscope.EndpointConfig, label string) {
	c := dashscope.NewTTSClient(key, model, voice, 22050, ep)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var n int
	err := c.Synthesize(ctx, "你好，我是 Mochi。", func(b []byte) { n += len(b) })
	fmt.Printf("[%s] %s / %s => err=%v bytes=%d\n", label, model, voice, err, n)
}

func main() {
	key := os.Getenv("DASHSCOPE_API_KEY")
	if key == "" {
		key = "sk-1a229ea079384e0e80caca71aa21a054"
	}
	wsID := os.Getenv("DASHSCOPE_WORKSPACE_ID")
	ep := dashscope.EndpointConfig{
		WorkspaceID: wsID,
		Region:      "cn-beijing",
	}
	if wsID != "" {
		ep.WSURL = dashscope.ResolveWSURL("", wsID, "cn-beijing")
	}
	cases := [][2]string{
		{"qwen-audio-3.0-tts-plus", "longanhuan_v3.6"},
		{"qwen-audio-3.0-tts-plus", "longanlingxi"},
		{"qwen-audio-3.0-tts-flash", "longanlingxi"},
	}
	label := "default"
	if wsID != "" {
		label = "cn-beijing:" + wsID
	}
	for _, x := range cases {
		try(key, x[0], x[1], ep, label)
	}
}
