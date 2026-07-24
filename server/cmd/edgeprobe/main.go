package main

import (
	"context"
	"fmt"
	"time"

	"github.com/difyz9/edge-tts-go/pkg/communicate"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comm, err := communicate.NewCommunicate(
		"你好，我是 Mochi。",
		"zh-CN-XiaoyiNeural",
		"+0%",
		"+0%",
		"+0Hz",
		"",
		10,
		60,
	)
	if err != nil {
		fmt.Println("new:", err)
		return
	}

	chunkChan, errChan := comm.Stream(ctx)
	var n int
	for chunk := range chunkChan {
		if chunk.Type == "audio" {
			n += len(chunk.Data)
		}
	}
	if err := <-errChan; err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Printf("ok bytes=%d\n", n)
}
