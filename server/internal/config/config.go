package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	AI       AIConfig       `yaml:"ai"`
	Client   ClientConfig   `yaml:"client"`
	Realtime RealtimeConfig `yaml:"realtime"`
	Companion CompanionConfig `yaml:"companion"`
	Growth    GrowthConfig    `yaml:"growth"`
	Tools     ToolsConfig     `yaml:"tools"`
}

type ServerConfig struct {
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	ReadTimeout  string `yaml:"read_timeout"`
	WriteTimeout string `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Name            string `yaml:"name"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
	AutoMigrate     bool   `yaml:"auto_migrate"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DB       int    `yaml:"db"`
	Password string `yaml:"password"`
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
	Expire string `yaml:"expire"`
	Issuer string `yaml:"issuer"`
}

type AIConfig struct {
	Name      string `yaml:"name"`
	APIBase   string `yaml:"api_base"`
	APIKey    string `yaml:"api_key"`
	ModelCode string `yaml:"model_code"`
}

type ClientConfig struct {
	APIBase string `yaml:"api_base"`
}

type RealtimeConfig struct {
	Enabled        bool                   `yaml:"enabled"`
	PrewarmEnabled bool                   `yaml:"prewarm_enabled"`
	Dashscope      RealtimeDashscope      `yaml:"dashscope"`
	VAD            RealtimeVAD            `yaml:"vad"`
	ASR            RealtimeASR            `yaml:"asr"`
	TTS            RealtimeTTS            `yaml:"tts"`
	Pipeline       RealtimePipeline       `yaml:"pipeline"`
	ThinkingFiller RealtimeThinkingFiller `yaml:"thinking_filler"`
}

type RealtimeDashscope struct {
	WorkspaceID string `yaml:"workspace_id"`
	Region      string `yaml:"region"`
	WSURL       string `yaml:"ws_url"`       // TTS 等业务空间端点
	ASRWSURL    string `yaml:"asr_ws_url"`   // 留空则用默认 dashscope 全球端点
}

type RealtimeVAD struct {
	Model              string `yaml:"model"`
	SilenceMS          int    `yaml:"silence_ms"`
	MinSpeechMS        int    `yaml:"min_speech_ms"`
	EndpointingEnabled bool   `yaml:"endpointing_enabled"`
}

type RealtimeASR struct {
	Provider   string `yaml:"provider"`
	Model      string `yaml:"model"`
	SampleRate int    `yaml:"sample_rate"`
}

type RealtimeTTS struct {
	Provider   string `yaml:"provider"`
	Model      string `yaml:"model"`
	Voice      string `yaml:"voice"`
	SampleRate int    `yaml:"sample_rate"`
}

type RealtimePipeline struct {
	TTSMinChars     int    `yaml:"tts_min_chars"`
	TTSPunctuation  string `yaml:"tts_punctuation"`
}

type RealtimeThinkingFiller struct {
	Enabled     bool     `yaml:"enabled"`
	ThresholdMS int      `yaml:"threshold_ms"`
	Phrases     []string `yaml:"phrases"`
}

type CompanionConfig struct {
	ProactiveEnabled   bool  `yaml:"proactive_enabled"`
	MaxDailyProactive  int   `yaml:"max_daily_proactive"`
	QuietHours         []int `yaml:"quiet_hours"`
	FollowUpEnabled    bool  `yaml:"follow_up_enabled"`
	MorningGreeting    bool  `yaml:"morning_greeting"`
	EveningGreeting    bool  `yaml:"evening_greeting"`
}

type GrowthConfig struct {
	Enabled                 bool `yaml:"enabled"`
	UserBriefCharBudget     int  `yaml:"user_brief_char_budget"`
	MemoryPromptCharBudget  int  `yaml:"memory_prompt_char_budget"`
	ReflectionEnabled       bool `yaml:"reflection_enabled"`
	ReflectionMinTurnChars  int  `yaml:"reflection_min_turn_chars"`
	WriteApproval           bool `yaml:"write_approval"`
	StyleEvolutionEnabled   bool `yaml:"style_evolution_enabled"`
	StyleEvolutionThreshold int  `yaml:"style_evolution_threshold"`
}

type ToolsConfig struct {
	Enabled              bool `yaml:"enabled"`
	MinRapportForSuggest int  `yaml:"min_rapport_for_suggest"`
	MinTrustForAutoCreate int `yaml:"min_trust_for_auto_create"`
	ReminderTickSeconds  int  `yaml:"reminder_tick_seconds"`
	MaxPendingReminders  int  `yaml:"max_pending_reminders"`
	RouterLLMEnabled     bool `yaml:"router_llm_enabled"`
}

func (c *Config) MySQLDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s&readTimeout=30s&writeTimeout=30s",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.Name)
}

func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}

func (c *Config) ServerPort() string {
	if c.Server.Port == 0 {
		return "8080"
	}
	return fmt.Sprintf("%d", c.Server.Port)
}

func (c *Config) ServerMode() string {
	if c.Server.Mode == "" {
		return "debug"
	}
	return c.Server.Mode
}

func Load() (*Config, error) {
	path, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if c.Database.Port == 0 {
		c.Database.Port = 3306
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 25
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 10
	}
	if c.Redis.Port == 0 {
		c.Redis.Port = 6379
	}
	if c.JWT.Secret == "" {
		c.JWT.Secret = "mochi-dev-secret"
	}
	if c.AI.ModelCode == "" && c.AI.Name != "" {
		c.AI.ModelCode = c.AI.Name
	}
	if c.Client.APIBase == "" {
		c.Client.APIBase = fmt.Sprintf("http://localhost:%d", c.Server.Port)
	}
	c.Realtime.applyDefaults()
	c.Companion.applyDefaults()
	c.Growth.applyDefaults()
	c.Tools.applyDefaults()
}

func (c *CompanionConfig) applyDefaults() {
	if c.MaxDailyProactive == 0 {
		c.MaxDailyProactive = 3
	}
	if len(c.QuietHours) == 0 {
		c.QuietHours = []int{23, 8}
	}
}

func (c *GrowthConfig) applyDefaults() {
	if c.UserBriefCharBudget == 0 {
		c.UserBriefCharBudget = 1400
	}
	if c.MemoryPromptCharBudget == 0 {
		c.MemoryPromptCharBudget = 400
	}
	if c.ReflectionMinTurnChars == 0 {
		c.ReflectionMinTurnChars = 4
	}
	if c.StyleEvolutionThreshold == 0 {
		c.StyleEvolutionThreshold = 3
	}
}

func (c *ToolsConfig) applyDefaults() {
	if c.MinRapportForSuggest == 0 {
		c.MinRapportForSuggest = 60
	}
	if c.MinTrustForAutoCreate == 0 {
		c.MinTrustForAutoCreate = 30
	}
	if c.ReminderTickSeconds == 0 {
		c.ReminderTickSeconds = 60
	}
	if c.MaxPendingReminders == 0 {
		c.MaxPendingReminders = 50
	}
}

func (r *RealtimeConfig) applyDefaults() {
	if r.VAD.SilenceMS == 0 {
		r.VAD.SilenceMS = 800
	}
	if r.VAD.MinSpeechMS == 0 {
		r.VAD.MinSpeechMS = 300
	}
	if r.VAD.Model == "" {
		r.VAD.Model = "energy"
	}
	if r.ThinkingFiller.ThresholdMS == 0 {
		r.ThinkingFiller.ThresholdMS = 800
	}
	if len(r.ThinkingFiller.Phrases) == 0 {
		r.ThinkingFiller.Phrases = []string{"嗯，让我想想~", "稍等一下哦~"}
	}
	if r.Dashscope.Region == "" {
		r.Dashscope.Region = "cn-beijing"
	}
	if r.ASR.Provider == "" {
		r.ASR.Provider = "dashscope"
	}
	if r.ASR.Model == "" {
		r.ASR.Model = "paraformer-realtime-v2"
	}
	if r.ASR.SampleRate == 0 {
		r.ASR.SampleRate = 16000
	}
	if r.TTS.Provider == "" {
		r.TTS.Provider = "dashscope"
	}
	if r.TTS.Model == "" {
		r.TTS.Model = "qwen-audio-3.0-tts-plus"
	}
	if r.TTS.Voice == "" {
		r.TTS.Voice = "longanhuan_v3.6"
	}
	if r.TTS.SampleRate == 0 {
		r.TTS.SampleRate = 22050
	}
	if r.Pipeline.TTSMinChars == 0 {
		r.Pipeline.TTSMinChars = 5
	}
	if r.Pipeline.TTSPunctuation == "" {
		r.Pipeline.TTSPunctuation = "。！？，、~.!?,;"
	}
}

func findConfigPath() (string, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("CONFIG_PATH not found: %s", p)
	}

	candidates := []string{
		"config.yaml",
		filepath.Join("..", "config.yaml"),
		filepath.Join("..", "..", "config.yaml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", fmt.Errorf("config.yaml not found (set CONFIG_PATH or place at project root)")
}

// ParseDuration helper for jwt expire etc.
func ParseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
