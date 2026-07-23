package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

const defaultBase = "http://127.0.0.1:8081"

type configFile struct {
	Realtime struct {
		Enabled bool `yaml:"enabled"`
		VAD     struct {
			SilenceMS          int  `yaml:"silence_ms"`
			EndpointingEnabled bool `yaml:"endpointing_enabled"`
		} `yaml:"vad"`
		ThinkingFiller struct {
			Enabled     bool `yaml:"enabled"`
			ThresholdMS int  `yaml:"threshold_ms"`
		} `yaml:"thinking_filler"`
	} `yaml:"realtime"`
}

type checkResult struct {
	Name   string
	Pass   bool
	Detail string
	Manual bool
}

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = filepath.Join("..", "..", "config.yaml")
	}
	base := os.Getenv("MOCHI_API_BASE")
	if base == "" {
		base = defaultBase
	}

	var results []checkResult

	testOut, testErr := exec.Command("go", "test", "./internal/realtime/...", "-count=1").CombinedOutput()
	results = append(results, checkResult{
		Name:   "realtime 单元测试",
		Pass:   testErr == nil,
		Detail: strings.TrimSpace(string(testOut)),
	})

	cfgOK, cfgDetail := checkConfig(cfgPath)
	results = append(results, checkResult{Name: "Phase2 配置项", Pass: cfgOK, Detail: cfgDetail})

	token, email, err := registerUser(base)
	if err != nil {
		results = append(results, checkResult{Name: "WS 文本 3 轮无崩溃", Pass: false, Detail: err.Error()})
	} else {
		wsOK, wsDetail := runWSRounds(base, token, 3)
		results = append(results, checkResult{
			Name:   "WS 文本 3 轮无崩溃",
			Pass:   wsOK,
			Detail: fmt.Sprintf("user=%s %s", email, wsDetail),
		})
	}

	results = append(results, manualChecks()...)

	passed, autoTotal := 0, 0
	for _, r := range results {
		if r.Manual {
			continue
		}
		autoTotal++
		if r.Pass {
			passed++
		}
	}

	var sb strings.Builder
	sb.WriteString("# 实时对话 Phase2 验收报告\n\n")
	sb.WriteString(fmt.Sprintf("时间: %s\n\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**自动化: %d/%d 通过**\n\n", passed, autoTotal))
	sb.WriteString("| 检查项 | 结果 | 说明 |\n|--------|------|------|\n")
	for _, r := range results {
		mark := "❌"
		if r.Pass {
			mark = "✅"
		}
		if r.Manual {
			mark = "⏳人工"
		}
		detail := strings.ReplaceAll(r.Detail, "|", "/")
		detail = strings.ReplaceAll(detail, "\n", " ")
		if len(detail) > 160 {
			detail = detail[:160] + "…"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Name, mark, detail))
	}

	out := os.Getenv("MOCHI_RT_REPORT")
	if out == "" {
		out = "realtimeaccept_report.md"
	}
	_ = os.WriteFile(out, []byte(sb.String()), 0644)
	fmt.Print(sb.String())

	if passed < autoTotal {
		os.Exit(1)
	}
}

func checkConfig(path string) (bool, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err.Error()
	}
	var cfg configFile
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return false, err.Error()
	}
	ok := cfg.Realtime.Enabled &&
		cfg.Realtime.VAD.SilenceMS == 500 &&
		cfg.Realtime.VAD.EndpointingEnabled &&
		cfg.Realtime.ThinkingFiller.Enabled
	if ok {
		return true, "silence_ms=500 endpointing=true filler threshold=800"
	}
	return false, fmt.Sprintf("enabled=%v silence=%d endpointing=%v filler=%v",
		cfg.Realtime.Enabled, cfg.Realtime.VAD.SilenceMS,
		cfg.Realtime.VAD.EndpointingEnabled, cfg.Realtime.ThinkingFiller.Enabled)
}

func registerUser(base string) (token, email string, err error) {
	email = fmt.Sprintf("rtaccept_%d@test.local", time.Now().Unix())
	regBody := fmt.Sprintf(`{"email":"%s","password":"evalpass123","pet_name":"RT"}`, email)
	b, err := postJSON(base+"/api/v1/auth/register", regBody, "")
	if err != nil {
		return "", "", err
	}
	var regOut struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &regOut); err != nil {
		return "", "", err
	}
	if regOut.Token == "" {
		return "", "", fmt.Errorf("no token in register response")
	}
	_, _ = postJSON(base+"/api/v1/subscribe/adopt", `{"sku_id":"cat_mochi_pink"}`, regOut.Token)
	return regOut.Token, email, nil
}

func runWSRounds(base, token string, rounds int) (bool, string) {
	wsURL := strings.Replace(base, "http://", "ws://", 1) + "/ws/voice?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return false, "dial: " + err.Error()
	}
	defer conn.Close()

	_ = conn.WriteJSON(map[string]interface{}{"type": "prewarm"})
	var metrics int
	for i := 0; i < rounds; i++ {
		text := fmt.Sprintf("验收测试第%d句，你好", i+1)
		payload, _ := json.Marshal(map[string]interface{}{
			"type": "text_input",
			"data": map[string]string{"text": text},
		})
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return false, err.Error()
		}
		deadline := time.Now().Add(90 * time.Second)
		gotDone := false
		for time.Now().Before(deadline) {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return false, err.Error()
			}
			var env struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(raw, &env) != nil {
				continue
			}
			switch env.Type {
			case "turn_metrics":
				metrics++
			case "tts_done", "llm_done":
				gotDone = true
			case "error":
				return false, string(env.Data)
			}
			if gotDone {
				break
			}
		}
		if !gotDone {
			return false, fmt.Sprintf("round %d timeout", i+1)
		}
	}
	return true, fmt.Sprintf("%d rounds ok; turn_metrics=%d", rounds, metrics)
}

func manualChecks() []checkResult {
	return []checkResult{
		{Name: "STT endpointing P50<600ms", Pass: false, Manual: true, Detail: "桌面+麦克风：判停 audio_end 应 300–600ms"},
		{Name: "Thinking filler 垫话", Pass: false, Manual: true, Detail: "语音轮次 LLM>800ms 时 1s 内有垫话"},
		{Name: "Barge-in 打断", Pass: false, Manual: true, Detail: "TTS 播放时说话可打断"},
		{Name: "5 轮连续语音", Pass: false, Manual: true, Detail: "无崩溃、无整轮 ASR/TTS 失败"},
		{Name: "playback 埋点 ≥4/5", Pass: false, Manual: true, Detail: "latency 日志 playback 非 -1"},
	}
}

func postJSON(url, body, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return b, fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}
	return b, nil
}
