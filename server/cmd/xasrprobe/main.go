// xasrprobe 探测服务端 x-asr sidecar 与 Go ASR 适配器是否可用。
//
// 用法:
//
//	go run ./cmd/xasrprobe
//	go run ./cmd/xasrprobe -ws ws://127.0.0.1:8766
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/mochi-ai/server/internal/realtime"
)

func main() {
	wsURL := flag.String("ws", "ws://127.0.0.1:8766", "x-asr WebSocket URL")
	sampleRate := flag.Int("sample-rate", 16000, "PCM sample rate")
	flag.Parse()

	asr := realtime.NewXasrASR(*wsURL, *sampleRate)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 生成 1 秒 440Hz 正弦波 int16 PCM（sidecar 可能识别为空，探测连通性即可）
	pcm := make([]byte, *sampleRate*2)
	for i := 0; i < *sampleRate; i++ {
		v := int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(*sampleRate)))
		pcm[i*2] = byte(v)
		pcm[i*2+1] = byte(v >> 8)
	}

	text, err := asr.Recognize(ctx, pcm, func(partial string, _ bool) {
		if partial != "" {
			fmt.Printf("partial: %q\n", partial)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK | ws=%s | final=%q\n", *wsURL, text)
}
