// 探测本地 X-TTS sidecar（Matcha HTTP :8767）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	base := os.Getenv("XTTS_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8767"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		fmt.Println("health req:", err)
		os.Exit(1)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("health:", err)
		os.Exit(1)
	}
	resp.Body.Close()
	fmt.Printf("health OK %s\n", base)

	body, _ := json.Marshal(map[string]any{"text": "你好，我是 Mochi。", "speed": 1.0})
	sreq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/synthesize", bytes.NewReader(body))
	if err != nil {
		fmt.Println("synth req:", err)
		os.Exit(1)
	}
	sreq.Header.Set("Content-Type", "application/json")
	sresp, err := http.DefaultClient.Do(sreq)
	if err != nil {
		fmt.Println("synth:", err)
		os.Exit(1)
	}
	defer sresp.Body.Close()
	raw, _ := io.ReadAll(sresp.Body)
	if sresp.StatusCode != http.StatusOK {
		fmt.Printf("synth http %d: %s\n", sresp.StatusCode, string(raw))
		os.Exit(1)
	}
	fmt.Printf("synth OK bytes=%d content-type=%s\n", len(raw), sresp.Header.Get("Content-Type"))
}
