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
	ASR      TencentConfig  `yaml:"asr"`
	TTS      TencentConfig  `yaml:"tts"`
	Client   ClientConfig   `yaml:"client"`
	Realtime RealtimeConfig `yaml:"realtime"`
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

type TencentConfig struct {
	AppID     string `yaml:"app_id"`
	SecretID  string `yaml:"secret_id"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	VoiceType int64  `yaml:"voice_type"`
}

type ClientConfig struct {
	APIBase string `yaml:"api_base"`
}

type RealtimeConfig struct {
	Enabled  bool            `yaml:"enabled"`
	VAD      RealtimeVAD     `yaml:"vad"`
	ASR      RealtimeASR     `yaml:"asr"`
	TTS      RealtimeTTS     `yaml:"tts"`
	Pipeline RealtimePipeline `yaml:"pipeline"`
}

type RealtimeVAD struct {
	Model        string `yaml:"model"`
	SilenceMS    int    `yaml:"silence_ms"`
	MinSpeechMS  int    `yaml:"min_speech_ms"`
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

func (c *Config) MySQLDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
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

func (c *Config) TTSConfig() TencentConfig {
	if c.TTS.SecretID != "" {
		return c.TTS
	}
	cfg := c.ASR
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	if cfg.VoiceType == 0 {
		cfg.VoiceType = 101001
	}
	return cfg
}

func (c *Config) ASRRegion() string {
	if c.ASR.Region != "" {
		return c.ASR.Region
	}
	return "ap-guangzhou"
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
		r.TTS.Model = "cosyvoice-v2"
	}
	if r.TTS.Voice == "" {
		r.TTS.Voice = "longxiaochun_v2"
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
