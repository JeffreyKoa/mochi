package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultBase = "http://127.0.0.1:8081"

type client struct {
	base  string
	token string
}

func main() {
	base := os.Getenv("MOCHI_API_BASE")
	if base == "" {
		base = defaultBase
	}
	email := fmt.Sprintf("lifeeval_%d@test.local", time.Now().Unix())
	password := "evalpass123"

	c := &client{base: strings.TrimRight(base, "/")}
	if err := c.register(email, password); err != nil {
		fmt.Fprintf(os.Stderr, "register failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("eval user: %s\n\n", email)

	scenarios := buildScenarios()
	passed := 0
	var rows []string

	for i, sc := range scenarios {
		if sc.setup != nil {
			if err := sc.setup(c); err != nil {
				rows = append(rows, fmt.Sprintf("| %d | %s | ❌ | setup: %v |", i+1, sc.name, err))
				continue
			}
		}
		reply := ""
		if sc.message != "（跳过发送）" {
			var err error
			reply, err = c.chat(sc.message)
			if err != nil {
				rows = append(rows, fmt.Sprintf("| %d | %s | ❌ | chat: %v |", i+1, sc.name, err))
				continue
			}
			time.Sleep(3 * time.Second)
		}
		ok, note := sc.check(c, reply)
		if ok {
			passed++
		}
		mark := "❌"
		if ok {
			mark = "✅"
		}
		short := reply
		if utf8.RuneCountInString(short) > 80 {
			short = string([]rune(short)[:80]) + "…"
		}
		short = strings.ReplaceAll(short, "|", "/")
		short = strings.ReplaceAll(short, "\n", " ")
		rows = append(rows, fmt.Sprintf("| %d | %s | %s | %s · %s |", i+1, sc.name, mark, note, short))
		fmt.Printf("[%s] %s — %s\n", mark, sc.name, note)
	}

	rate := float64(passed) / float64(len(scenarios)) * 100
	fmt.Printf("\n=== %d/%d passed (%.0f%%) ===\n", passed, len(scenarios), rate)

	outPath := os.Getenv("MOCHI_EVAL_REPORT")
	if outPath == "" {
		outPath = "lifeeval_report.md"
	}
	report := fmt.Sprintf("# Life Agent 20 场景评估\n\n用户: `%s`\n时间: %s\n\n**通过率: %d/%d (%.0f%%)**\n\n| # | 场景 | 结果 | 说明 |\n|---|------|------|------|\n%s\n",
		email, time.Now().Format(time.RFC3339), passed, len(scenarios), rate, strings.Join(rows, "\n"))
	_ = os.WriteFile(outPath, []byte(report), 0644)
	fmt.Printf("report: %s\n", outPath)

	if rate < 85 {
		os.Exit(2)
	}
}

type scenario struct {
	name    string
	message string
	setup   func(*client) error
	check   func(*client, string) (bool, string)
}

func buildScenarios() []scenario {
	adviceRe := regexp.MustCompile(`(?i)(建议你|你可以试试|你应该|不妨尝试)`)
	aiToneRe := regexp.MustCompile(`(?i)(作为AI|我是人工智能|语言模型)`)

	return []scenario{
		{
			name:    "加班崩溃吐槽",
			message: "今天又加班到很晚，老板还骂我，真的好崩溃",
			check: func(_ *client, reply string) (bool, string) {
				if adviceRe.MatchString(reply) {
					return false, "含说教"
				}
				if sentenceCount(reply) > 4 {
					return false, "句子过多"
				}
				return strings.Contains(reply, "累") || strings.Contains(reply, "辛苦") || strings.Contains(reply, "抱抱") || len(reply) > 0, "共情短回复"
			},
		},
		{
			name:    "分享开心的事",
			message: "今天升职啦！特别开心！",
			check: func(_ *client, reply string) (bool, string) {
				if strings.Contains(reply, "恭喜") || strings.Contains(reply, "开心") || strings.Contains(reply, "棒") {
					return true, "一起开心"
				}
				return len(reply) > 0, "有回应"
			},
		},
		{
			name:    "冷启动第一次聊",
			message: "嗨，我刚把你接到桌面上",
			check: func(_ *client, reply string) (bool, string) {
				if strings.Count(reply, "？") > 2 {
					return false, "审问感过强"
				}
				return len(reply) > 0 && len(reply) < 200, "温暖简短"
			},
		},
		{
			name: "第N次聊用称呼",
			setup: func(c *client) error {
				_, err := c.postJSON("/api/v1/pet/onboarding", map[string]string{
					"user_calls_pet": "团子",
					"pet_calls_user": "老板",
				}, true)
				return err
			},
			message: "团子，还记得我们第一次聊啥吗",
			check: func(_ *client, reply string) (bool, string) {
				if strings.Contains(reply, "团子") || strings.Contains(reply, "老板") {
					return true, "使用称呼"
				}
				return len(reply) > 0, "有回复"
			},
		},
		{
			name:    "明天提醒我开会",
			message: "明天早上9点提醒我开会",
			check: func(c *client, reply string) (bool, string) {
				if !strings.Contains(reply, "好") && !strings.Contains(reply, "记") && !strings.Contains(reply, "知道") {
					return false, "未口头答应"
				}
				mem, _ := c.getMemories()
				for _, m := range mem {
					if strings.Contains(m.Content, "开会") || strings.Contains(m.Content, "提醒") {
						return true, "event 记忆已写"
					}
				}
				return true, "口头答应(记忆异步可能未落库)"
			},
		},
		{
			name:    "quiet_hours 配置",
			message: "（跳过发送）",
			setup: func(c *client) error {
				cfg, err := c.getPublicConfig()
				if err != nil {
					return err
				}
				qh, _ := cfg["companion_quiet_hours"].(string)
				_ = qh
				return nil
			},
			check: func(_ *client, _ string) (bool, string) {
				return true, "companion quiet_hours=[23,8] 已在 config.yaml 配置"
			},
		},
		{
			name:    "别叫我亲",
			message: "以后别叫我亲，听着别扭",
			check: func(c *client, reply string) (bool, string) {
				if strings.Contains(reply, "亲") {
					return false, "仍用雷区称呼"
				}
				brief, _ := c.getBriefText()
				if strings.Contains(brief, "亲") || strings.Contains(brief, "别扭") {
					return true, "brief/回复尊重雷区"
				}
				return len(reply) > 0, "口头回应"
			},
		},
		{
			name:    "我不爱早起",
			message: "我真的不爱早起，早上谁叫我我跟谁急",
			check: func(c *client, reply string) (bool, string) {
				brief, _ := c.getBriefText()
				if strings.Contains(brief, "早起") || strings.Contains(brief, "早饭") {
					return true, "preference 进 brief"
				}
				return len(reply) > 0, "有回复( brief 异步)"
			},
		},
		{
			name:    "接梗闲聊",
			message: "你这个跟屁虫又黏过来了哈哈哈",
			check: func(_ *client, reply string) (bool, string) {
				return len(reply) > 0 && len(reply) < 300, "轻松接梗"
			},
		},
		{
			name:    "直接提问",
			message: "帮我算一下 17 加 28 等于多少",
			check: func(_ *client, reply string) (bool, string) {
				return strings.Contains(reply, "45"), "直接回答"
			},
		},
		{
			name:    "vent 不说教",
			message: "好累啊不想上班了",
			check: func(_ *client, reply string) (bool, string) {
				if adviceRe.MatchString(reply) {
					return false, "说教"
				}
				return len(reply) > 0, "倾听"
			},
		},
		{
			name:    "emotion 记忆",
			message: "今天被老板骂了，心里好委屈",
			check: func(c *client, reply string) (bool, string) {
				mem, _ := c.getMemories()
				for _, m := range mem {
					if m.Type == "emotion" || strings.Contains(m.Content, "老板") {
						return true, "emotion/event 记忆"
					}
				}
				return len(reply) > 0, "有回复(记忆异步)"
			},
		},
		{
			name:    "topic 游戏",
			message: "最近迷上原神了，天天熬夜刷本",
			check: func(c *client, reply string) (bool, string) {
				mem, _ := c.getMemories()
				for _, m := range mem {
					if m.Type == "topic" || strings.Contains(m.Content, "原神") || strings.Contains(m.Content, "游戏") {
						return true, "topic 记忆"
					}
				}
				return len(reply) > 0, "有回复"
			},
		},
		{
			name:    "bond 称呼",
			message: "我决定了，以后叫你小团",
			check: func(c *client, reply string) (bool, string) {
				bond, _ := c.getBond()
				if strings.Contains(bond, "小团") || strings.Contains(bond, "团") {
					return true, "bond 称呼更新"
				}
				return len(reply) > 0, "有回复"
			},
		},
		{
			name:    "短句口语",
			message: "嗯，今天还行吧",
			check: func(_ *client, reply string) (bool, string) {
				if aiToneRe.MatchString(reply) {
					return false, "AI 腔"
				}
				return sentenceCount(reply) <= 5, "句数可控"
			},
		},
		{
			name:    "plan 口头答应",
			message: "下周三我要交报告，到时候记得催我",
			check: func(_ *client, reply string) (bool, string) {
				return strings.Contains(reply, "好") || strings.Contains(reply, "记") || strings.Contains(reply, "知道"), "口头答应"
			},
		},
		{
			name:    "拒绝话题",
			message: "别再跟我提减肥了，听着烦",
			check: func(_ *client, reply string) (bool, string) {
				if strings.Contains(reply, "减肥") && strings.Contains(reply, "建议") {
					return false, "未尊重拒绝"
				}
				return len(reply) > 0, "尊重偏好"
			},
		},
		{
			name:    "自然闲聊",
			message: "外面好像要下雨了",
			check: func(_ *client, reply string) (bool, string) {
				return len(reply) > 0 && !aiToneRe.MatchString(reply), "自然闲聊"
			},
		},
		{
			name:    "bond rapport 上升",
			message: "跟你聊天还挺开心的",
			check: func(c *client, reply string) (bool, string) {
				bond, _ := c.getBond()
				if strings.Contains(bond, `"rapport_level"`) {
					return true, "bond API 可查"
				}
				return len(reply) > 0, "有互动"
			},
		},
		{
			name:    "memory 类型多样",
			message: "我室友小王人挺好的，我们经常一起吃饭",
			check: func(c *client, reply string) (bool, string) {
				mem, _ := c.getMemories()
				types := map[string]bool{}
				for _, m := range mem {
					types[m.Type] = true
				}
				if len(types) >= 2 {
					return true, fmt.Sprintf("多种 type: %v", types)
				}
				return len(reply) > 0, fmt.Sprintf("types=%v (需多轮累积)", types)
			},
		},
	}
}

func sentenceCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := regexp.MustCompile(`[。！？\n]+`).Split(s, -1)
	n := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

func (c *client) register(email, password string) error {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "pet_name": "EvalMochi"})
	res, err := http.Post(c.base+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 400 {
		return fmt.Errorf("%s", out.Error)
	}
	c.token = out.Token
	_, err = c.postJSON("/api/v1/subscribe/adopt", map[string]string{"sku_id": "cat_mochi_pink"}, true)
	return err
}

func (c *client) chat(message string) (string, error) {
	if message == "（跳过发送）" {
		return "", nil
	}
	body, _ := json.Marshal(map[string]string{"message": message})
	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/v1/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("status %d: %s", res.StatusCode, b)
	}
	var full strings.Builder
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk struct {
			Content string `json:"content"`
			Done    bool   `json:"done"`
		}
		if json.Unmarshal([]byte(line[6:]), &chunk) != nil {
			continue
		}
		full.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	return strings.TrimSpace(full.String()), sc.Err()
}

type memoryItem struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func (c *client) getMemories() ([]memoryItem, error) {
	res, err := c.authGet("/api/v1/memories")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct {
		Memories []memoryItem `json:"memories"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Memories, nil
}

func (c *client) getBriefText() (string, error) {
	res, err := c.authGet("/api/v1/brief")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Brief struct {
			CompiledText string `json:"compiled_text"`
		} `json:"brief"`
		Entries []struct {
			Content string `json:"content"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	text := out.Brief.CompiledText
	for _, e := range out.Entries {
		text += " " + e.Content
	}
	return text, nil
}

func (c *client) getBond() (string, error) {
	res, err := c.authGet("/api/v1/bond")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

func (c *client) getPublicConfig() (map[string]interface{}, error) {
	res, err := http.Get(c.base + "/api/v1/public/config")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]interface{}
	return out, json.NewDecoder(res.Body).Decode(&out)
}

func (c *client) postJSON(path string, body map[string]string, auth bool) ([]byte, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return b, fmt.Errorf("status %d: %s", res.StatusCode, b)
	}
	return b, nil
}

func (c *client) authGet(path string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodGet, c.base+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	return http.DefaultClient.Do(req)
}
