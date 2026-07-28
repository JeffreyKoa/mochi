package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GateFastpath holds gate fast-path keyword lists (loaded from config/data).
type GateFastpath struct {
	QuestionWords []string `yaml:"question_words"`
	AddressWords  []string `yaml:"address_words"`
	ShareWords    []string `yaml:"share_words"`
}

func (g GateFastpath) ApplyDefaults() GateFastpath {
	out := g
	if len(out.QuestionWords) == 0 {
		out.QuestionWords = []string{
			"吗", "呢", "什么", "为什么", "怎么", "怎样", "哪", "谁", "多少", "几", "吗？", "呢？",
		}
	}
	if len(out.AddressWords) == 0 {
		out.AddressWords = []string{
			"你好", "在吗", "在不在", "喂", "嗨", "hello", "hi",
			"过来", "说话", "回答", "帮我", "请你", "麻烦",
			"好", "对", "行", "知道", "想", "聊", "听", "没",
			"好的", "是的", "可以", "对啊", "拜拜", "再见", "摸摸", "看看",
		}
	}
	if len(out.ShareWords) == 0 {
		out.ShareWords = []string{
			"准备", "打算", "今天", "刚才", "刚刚", "觉得", "感觉", "好像", "可能",
			"告诉", "跟你说", "吃饭", "午饭", "晚饭", "早餐", "去了", "回来",
			"开始", "完成", "有点", "开心", "难过", "累", "忙",
		}
	}
	return out
}

type noiseFillersYAML struct {
	Fillers []string `yaml:"fillers"`
}

const defaultGateSystemPrompt = `你是语音助手的回应判断器。判断用户这句话是否在对桌面宠物说话且需要回应。
自言自语、对别人说话、无意义碎片、背景对话 → respond=false；
提问、指令、问候、分享、闲聊 → respond=true。
只输出JSON {"respond":true} 或 {"respond":false}，不要输出任何其他内容。`

func defaultNoiseFillerRunes() map[rune]bool {
	words := []string{
		"啊", "呃", "哦", "唉", "哎", "哼", "咳", "额", "噢", "喔", "呀",
		"哈", "嘿", "唔", "呐", "嗐",
	}
	return BuildNoiseFillerSet(words)
}

// BuildNoiseFillerSet converts filler strings into a rune lookup set.
func BuildNoiseFillerSet(words []string) map[rune]bool {
	out := make(map[rune]bool, len(words))
	for _, w := range words {
		for _, r := range w {
			out[r] = true
		}
	}
	return out
}

func (c *Config) loadDataFiles(configDir string) error {
	c.configDir = configDir

	fastpathPath := c.resolveDataPath(configDir, c.Realtime.Gate.FastpathFile, "data/gate_fastpath.yaml")
	fastpath, err := loadGateFastpath(fastpathPath)
	if err != nil {
		return err
	}
	c.GateFastpath = fastpath.ApplyDefaults()

	promptPath := c.resolveDataPath(configDir, c.Realtime.Gate.SystemPromptFile, "data/gate_system_prompt.txt")
	prompt, err := loadTextFile(promptPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		c.GateSystemPrompt = defaultGateSystemPrompt
	} else {
		c.GateSystemPrompt = strings.TrimSpace(prompt)
	}

	fillersPath := c.resolveDataPath(configDir, c.Realtime.ASR.NoiseFillersFile, "data/asr_noise_fillers.yaml")
	fillers, err := loadNoiseFillers(fillersPath)
	if err != nil {
		return err
	}
	if len(fillers) == 0 {
		c.NoiseFillers = defaultNoiseFillerRunes()
	} else {
		c.NoiseFillers = fillers
	}
	return nil
}

func (c *Config) resolveDataPath(configDir, configured, defaultRel string) string {
	rel := strings.TrimSpace(configured)
	if rel == "" {
		rel = defaultRel
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(configDir, rel)
}

// ConfigDir returns the directory containing the loaded config.yaml.
func (c *Config) ConfigDir() string {
	return c.configDir
}

func loadGateFastpath(path string) (GateFastpath, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GateFastpath{}, nil
		}
		return GateFastpath{}, fmt.Errorf("read gate fastpath %s: %w", path, err)
	}
	var out GateFastpath
	if err := yaml.Unmarshal(data, &out); err != nil {
		return GateFastpath{}, fmt.Errorf("parse gate fastpath %s: %w", path, err)
	}
	return out, nil
}

func loadNoiseFillers(path string) (map[rune]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read noise fillers %s: %w", path, err)
	}
	var raw noiseFillersYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse noise fillers %s: %w", path, err)
	}
	if len(raw.Fillers) == 0 {
		return nil, nil
	}
	return BuildNoiseFillerSet(raw.Fillers), nil
}

func loadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
