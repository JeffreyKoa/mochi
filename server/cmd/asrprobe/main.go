package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mochi-ai/server/pkg/dashscope"
)

func test(label string, ep dashscope.EndpointConfig) {
	c := dashscope.NewASRClient("sk-1a229ea079384e0e80caca71aa21a054", "paraformer-realtime-v2", 16000, ep)
	pcm := make([]byte, 32000)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := c.Recognize(ctx, pcm, nil)
	fmt.Printf("%s => err=%v\n", label, err)
}

func main() {
	test("default", dashscope.EndpointConfig{})
	test("workspace", dashscope.EndpointConfig{
		WSURL:       "wss://llm-r716jer8vq4n6cyo.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference",
		WorkspaceID: "llm-r716jer8vq4n6cyo",
	})
}
