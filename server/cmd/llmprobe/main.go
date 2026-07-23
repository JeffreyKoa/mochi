// Quick LLM latency probe — reads config.yaml via CONFIG_PATH.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/config"
)

const probePrompt = "主人说：今天有点累。请用 Mochi 的语气简短安慰两句，不超过40字。"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	model := cfg.AI.ModelCode
	base := strings.TrimRight(cfg.AI.APIBase, "/")
	fmt.Printf("model=%s base=%s\n", model, base)

	runs := 3
	var ttfts, totals []time.Duration

	for i := 1; i <= runs; i++ {
		ttft, total, content, err := streamOnce(context.Background(), base, cfg.AI.APIKey, model, probePrompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run %d error: %v\n", i, err)
			continue
		}
		ttfts = append(ttfts, ttft)
		totals = append(totals, total)
		fmt.Printf("run %d: ttft=%dms total=%dms chars=%d reply=%q\n",
			i, ttft.Milliseconds(), total.Milliseconds(), len([]rune(content)), truncate(content, 60))
		time.Sleep(500 * time.Millisecond)
	}

	if len(ttfts) == 0 {
		os.Exit(1)
	}
	fmt.Printf("\navg ttft=%dms avg total=%dms (n=%d)\n",
		avg(ttfts).Milliseconds(), avg(totals).Milliseconds(), len(ttfts))
}

func streamOnce(ctx context.Context, base, apiKey, model, prompt string) (ttft, total time.Duration, content string, err error) {
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.8,
		"stream":      true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, 0, "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}

	var sb strings.Builder
	first := true
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		if first {
			ttft = time.Since(start)
			first = false
		}
		sb.WriteString(chunk.Choices[0].Delta.Content)
	}
	total = time.Since(start)
	return ttft, total, sb.String(), sc.Err()
}

func avg(d []time.Duration) time.Duration {
	var s time.Duration
	for _, v := range d {
		s += v
	}
	return s / time.Duration(len(d))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
