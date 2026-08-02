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
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	AI       AIConfig       `yaml:"ai"`
	Client   ClientConfig   `yaml:"client"`
	Realtime RealtimeConfig `yaml:"realtime"`
	Companion CompanionConfig `yaml:"companion"`
	Growth    GrowthConfig    `yaml:"growth"`
	Tools     ToolsConfig     `yaml:"tools"`
	Wellness  WellnessConfig  `yaml:"wellness"`
	Emotion   EmotionConfig   `yaml:"emotion"`
	Vision    VisionConfig    `yaml:"vision"`

	// Loaded from config/data/* (not in main yaml).
	configDir        string
	GateFastpath     GateFastpath
	GateSystemPrompt string
	NoiseFillers     map[rune]bool
}

type ServerConfig struct {
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	ReadTimeout  string `yaml:"read_timeout"`
	WriteTimeout string `yaml:"write_timeout"`
}

// LogConfig controls daily log file output (logs/mochi-YYYYMMDD.log).
type LogConfig struct {
	Dir string `yaml:"dir"`
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
	Name            string `yaml:"name"`
	APIBase         string `yaml:"api_base"`
	APIKey          string `yaml:"api_key"`
	ModelCode       string `yaml:"model_code"`
	EnableSearch    bool   `yaml:"enable_search"`
	SearchStrategy  string `yaml:"search_strategy"` // turbo | max
}

type ClientConfig struct {
	APIBase string `yaml:"api_base"`
}

type RealtimeConfig struct {
	Enabled        bool                   `yaml:"enabled"`
	STTMode        string                 `yaml:"stt_mode"` // cloud | local | auto
	PrewarmEnabled bool                   `yaml:"prewarm_enabled"`
	Dashscope      RealtimeDashscope      `yaml:"dashscope"`
	VAD            RealtimeVAD            `yaml:"vad"`
	BargeIn        RealtimeBargeIn        `yaml:"barge_in"`
	ASR            RealtimeASR            `yaml:"asr"`
	TTS            RealtimeTTS            `yaml:"tts"`
	Pipeline       RealtimePipeline       `yaml:"pipeline"`
	ThinkingFiller RealtimeThinkingFiller `yaml:"thinking_filler"`
	Gate           RealtimeGate           `yaml:"gate"`
	Voiceprint     RealtimeVoiceprint     `yaml:"voiceprint"`
	Faceprint      RealtimeFaceprint      `yaml:"faceprint"`
	Presence       RealtimePresence       `yaml:"presence"`
	TopicAnchor    RealtimeTopicAnchor    `yaml:"topic_anchor"`
}

type RealtimeDashscope struct {
	WorkspaceID string `yaml:"workspace_id"`
	Region      string `yaml:"region"`
	WSURL       string `yaml:"ws_url"`       // TTS 等业务空间端点
	ASRWSURL    string `yaml:"asr_ws_url"`   // 留空则用默认 dashscope 全球端点
}

type RealtimeVAD struct {
	Model              string            `yaml:"model"`
	SilenceMS          int               `yaml:"silence_ms"`
	MinSpeechMS        int               `yaml:"min_speech_ms"`
	EndpointingEnabled bool              `yaml:"endpointing_enabled"`
	EnergyPeak         float64           `yaml:"energy_peak"`
	TailSpeechPeak     float64           `yaml:"tail_speech_peak"`
	PlaybackPeak       float64           `yaml:"playback_peak"`
	WakePeak           float64           `yaml:"wake_peak"`
	Silero             RealtimeSileroVAD `yaml:"silero"`
}

// RealtimeSileroVAD tunes @ricky0123/vad-web on the desktop client.
type RealtimeSileroVAD struct {
	PositiveThreshold float64 `yaml:"positive_threshold"`
	NegativeThreshold float64 `yaml:"negative_threshold"`
	RedemptionMS      int     `yaml:"redemption_ms"`
	MinSpeechMS       int     `yaml:"min_speech_ms"`
	PreSpeechPadMS    int     `yaml:"pre_speech_pad_ms"`
}

type RealtimeBargeIn struct {
	EchoGuardMS    int     `yaml:"echo_guard_ms"`
	EchoGuardMSAEC int     `yaml:"echo_guard_ms_aec"` // AEC 握手成功后的缩短窗口
	PeakThreshold  float64 `yaml:"peak_threshold"`
	BargeInMS      int     `yaml:"barge_in_ms"`
}

type RealtimeVoiceprint struct {
	Required                 bool    `yaml:"required"`
	Threshold                float64 `yaml:"threshold"`
	VerifyWindowSec          float64 `yaml:"verify_window_sec"`
	WakeProbeSec             float64 `yaml:"wake_probe_sec"`
	StreamCheckIntervalMS    int     `yaml:"stream_check_interval_ms"`
	RejectStreak             int     `yaml:"reject_streak"`
	OwnerRecentMS            int     `yaml:"owner_recent_ms"`
	NonOwnerReplyCooldownMS  int     `yaml:"non_owner_reply_cooldown_ms"`
}

// RealtimeFaceprint 主人人脸识别（P2，客户端 ONNX）。
type RealtimeFaceprint struct {
	Enabled            bool    `yaml:"enabled"`
	Required           bool    `yaml:"required"`
	MatchThreshold     float64 `yaml:"match_threshold"`
	GrayZoneLow        float64 `yaml:"gray_zone_low"`
	ProbeOnSpeechStart bool    `yaml:"probe_on_speech_start"`
	CheckIntervalMS    int     `yaml:"check_interval_ms"`
	OwnerRecentMS      int     `yaml:"owner_recent_ms"`
	EnrollSamples      int     `yaml:"enroll_samples"`
}

type RealtimePresence struct {
	Enabled             bool    `yaml:"enabled"`
	AmbientIntervalMS   int     `yaml:"ambient_interval_ms"`
	AwayTimeoutSec      int     `yaml:"away_timeout_sec"`
	SpeechThreshold     float64 `yaml:"speech_threshold"`
	AmbientEnergyFloor  float64 `yaml:"ambient_energy_floor"`
	OwnerPresenceTTLSec int     `yaml:"owner_presence_ttl_sec"`
}

// RealtimeTopicAnchor 会话级话题锚点（P1）。
type RealtimeTopicAnchor struct {
	Enabled     bool `yaml:"enabled"`
	StickyTurns int  `yaml:"sticky_turns"`
}

// RealtimePublicConfig is exposed via GET /api/v1/public/config (no secrets).
type RealtimePublicConfig struct {
	STTMode       string `json:"stt_mode"`
	SpeechLocale  string `json:"speech_locale"`
	VAD           struct {
		SilenceMS          int     `json:"silence_ms"`
		MinSpeechMS        int     `json:"min_speech_ms"`
		EndpointingEnabled bool    `json:"endpointing_enabled"`
		EnergyPeak         float64 `json:"energy_peak"`
		TailSpeechPeak     float64 `json:"tail_speech_peak"`
		PlaybackPeak       float64 `json:"playback_peak"`
		WakePeak           float64 `json:"wake_peak"`
		Silero             struct {
			PositiveThreshold float64 `json:"positive_threshold"`
			NegativeThreshold float64 `json:"negative_threshold"`
			RedemptionMS      int     `json:"redemption_ms"`
			MinSpeechMS       int     `json:"min_speech_ms"`
			PreSpeechPadMS    int     `json:"pre_speech_pad_ms"`
		} `json:"silero"`
	} `json:"vad"`
	BargeIn struct {
		EchoGuardMS   int     `json:"echo_guard_ms"`
		PeakThreshold float64 `json:"peak_threshold"`
		BargeInMS     int     `json:"barge_in_ms"`
	} `json:"barge_in"`
	Voiceprint struct {
		Required                bool    `json:"required"`
		Threshold               float64 `json:"threshold"`
		VerifyWindowSec         float64 `json:"verify_window_sec"`
		WakeProbeSec            float64 `json:"wake_probe_sec"`
		StreamCheckIntervalMS   int     `json:"stream_check_interval_ms"`
		RejectStreak            int     `json:"reject_streak"`
		OwnerRecentMS           int     `json:"owner_recent_ms"`
		NonOwnerReplyCooldownMS int     `json:"non_owner_reply_cooldown_ms"`
	} `json:"voiceprint"`
	Faceprint struct {
		Enabled            bool    `json:"enabled"`
		Required           bool    `json:"required"`
		MatchThreshold     float64 `json:"match_threshold"`
		GrayZoneLow        float64 `json:"gray_zone_low"`
		ProbeOnSpeechStart bool    `json:"probe_on_speech_start"`
		CheckIntervalMS    int     `json:"check_interval_ms"`
		OwnerRecentMS      int     `json:"owner_recent_ms"`
		EnrollSamples      int     `json:"enroll_samples"`
	} `json:"faceprint"`
	Presence struct {
		Enabled            bool    `json:"enabled"`
		AmbientIntervalMS  int     `json:"ambient_interval_ms"`
		AwayTimeoutSec     int     `json:"away_timeout_sec"`
		SpeechThreshold    float64 `json:"speech_threshold"`
		AmbientEnergyFloor float64 `json:"ambient_energy_floor"`
		OwnerPresenceTTLSec int    `json:"owner_presence_ttl_sec"`
	} `json:"presence"`
	TTSTransport string `json:"tts_transport"`
}

type RealtimeASR struct {
	Provider         string `yaml:"provider"`
	Model            string `yaml:"model"`
	SampleRate       int    `yaml:"sample_rate"`
	NoiseFillersFile string `yaml:"noise_fillers_file"`
}

type RealtimeTTS struct {
	Provider   string             `yaml:"provider"`
	Fallback   string             `yaml:"fallback"`
	Model      string             `yaml:"model"`
	Voice      string             `yaml:"voice"`
	SampleRate int                `yaml:"sample_rate"`
	Transport  string             `yaml:"transport"` // mp3 | opus
	Opus       RealtimeOpusConfig `yaml:"opus"`
}

type RealtimeOpusConfig struct {
	SampleRate  int    `yaml:"sample_rate"`
	Bitrate     int    `yaml:"bitrate"`
	FrameMS     int    `yaml:"frame_ms"`
	Application string `yaml:"application"`
}

type RealtimePipeline struct {
	TTSMinChars            int    `yaml:"tts_min_chars"`
	TTSFirstMinChars       int    `yaml:"tts_first_min_chars"`
	TTSWeakPunctMinChars   int    `yaml:"tts_weak_punct_min_chars"`
	TTSForceFlushChars     int    `yaml:"tts_force_flush_chars"`
	TTSPunctuation         string `yaml:"tts_punctuation"`
	TTSStrongPunctuation   string `yaml:"tts_strong_punctuation"`
	TTSWeakPunctuation     string `yaml:"tts_weak_punctuation"`
}

type RealtimeThinkingFiller struct {
	Enabled     bool     `yaml:"enabled"`
	ThresholdMS int      `yaml:"threshold_ms"`
	Phrases     []string `yaml:"phrases"`
}

// RealtimeGate configures the lightweight LLM response gate that filters
// ASR transcripts before they reach the chat LLM (voice turns only).
type RealtimeGate struct {
	Enabled          bool   `yaml:"enabled"`
	Model            string `yaml:"model"`
	TimeoutMS        int    `yaml:"timeout_ms"`
	MaxChars         int    `yaml:"max_chars"`
	MaxTokens        int    `yaml:"max_tokens"`
	FastpathFile     string `yaml:"fastpath_file"`
	SystemPromptFile string `yaml:"system_prompt_file"`
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

type WellnessConfig struct {
	Enabled                   bool              `yaml:"enabled"`
	TickMinutes               int               `yaml:"tick_minutes"`
	MaxDailyNudges            int               `yaml:"max_daily_nudges"`
	ActivityIdleBreakMinutes  int               `yaml:"activity_idle_break_minutes"`
	DrinkActiveMinutes        int               `yaml:"drink_active_minutes"`
	RestActiveMinutes         int               `yaml:"rest_active_minutes"`
	OverworkActiveMinutes     int               `yaml:"overwork_active_minutes"`
	MealWindows               map[string][]string `yaml:"meal_windows"`
	EveningRestHour           int               `yaml:"evening_rest_hour"`
}

// EmotionConfig 情绪感知相关配置。
type EmotionConfig struct {
	Acoustic AcousticEmotionConfig `yaml:"acoustic"`
}

// AcousticEmotionConfig emotion2vec 旁路微服务配置。
type AcousticEmotionConfig struct {
	Enabled       bool    `yaml:"enabled"`
	URL           string  `yaml:"url"`
	TimeoutMS     int     `yaml:"timeout_ms"`
	MinConfidence float64 `yaml:"min_confidence"`
}

func (e *EmotionConfig) applyDefaults() {
	if e.Acoustic.TimeoutMS == 0 {
		e.Acoustic.TimeoutMS = 800
	}
	if e.Acoustic.MinConfidence == 0 {
		e.Acoustic.MinConfidence = 0.65
	}
}

// VisionConfig 视觉感知（Qwen-VL，V1 owner_face + V1.5 object/scene 路由 + V3 并行/早推）。
type VisionConfig struct {
	Enabled                 bool     `yaml:"enabled"`
	Model                   string   `yaml:"model"`
	TimeoutMS               int      `yaml:"timeout_ms"`
	DefaultFocus            string   `yaml:"default_focus"`
	AutoOnVoiceTurn         bool     `yaml:"auto_on_voice_turn"`
	MinExpressionConfidence float64  `yaml:"min_expression_confidence"`
	ObjectTriggerKeywords   []string `yaml:"object_trigger_keywords"`
	SceneTriggerKeywords    []string `yaml:"scene_trigger_keywords"`
	// V3a：vision_frame 到达即预跑 owner_face；false 则仅在 audio_end 并行。
	PrefetchOnFrame bool `yaml:"prefetch_on_frame"`
	// V3a：true 时 owner_face 与 ASR 串行（降级）；默认 false = 并行。
	SequentialOwnerFace bool `yaml:"sequential_owner_face"`
	// V3b：感知先到先推动画（LLM 之前）。
	EarlyAnimation bool `yaml:"early_animation"`
	// V3b：早推动画最低视觉置信（与 MinExpressionConfidence 取 max）。
	EarlyAnimationMinConf float64 `yaml:"early_animation_min_conf"`
	// V3c：Contextual Planner + 语义分类驱动二次 VL（禁用关键词路由）。
	ContextualPlanner bool `yaml:"contextual_planner"`
	ClassifyEnabled   bool `yaml:"classify_enabled"`
	ClassifyModel     string `yaml:"classify_model"`
	ClassifyTimeoutMS int    `yaml:"classify_timeout_ms"`
	// P0：会话级摄像头（客户端 startTalk 后保持流，turn 内快拍）。
	SessionCamera bool `yaml:"session_camera"`
	// P1：speech_start 时预拍一帧，拉长 prefetch 窗口。
	SnapshotOnSpeechStart bool `yaml:"snapshot_on_speech_start"`
	// audio_end 时抓拍（默认 true）。
	SnapshotOnAudioEnd bool `yaml:"snapshot_on_audio_end"`
	// P2：举物语义 detected 时客户端补拍 object_refresh 帧。
	SnapshotOnObjectIntent bool `yaml:"snapshot_on_object_intent"`
}

// VisionPublicConfig 暴露给客户端的视觉配置（无密钥）。
type VisionPublicConfig struct {
	SessionCamera          bool `json:"session_camera"`
	SnapshotOnSpeechStart  bool `json:"snapshot_on_speech_start"`
	SnapshotOnAudioEnd     bool `json:"snapshot_on_audio_end"`
	SnapshotOnObjectIntent bool `json:"snapshot_on_object_intent"`
}

// PublicClient 返回客户端可用的视觉开关。
func (v *VisionConfig) PublicClient() VisionPublicConfig {
	sessionCam := v.SessionCamera
	snapSpeech := v.SnapshotOnSpeechStart
	snapAudioEnd := v.SnapshotOnAudioEnd
	snapObject := v.SnapshotOnObjectIntent
	if v.Enabled {
		if !sessionCam && !snapSpeech && !snapAudioEnd && !snapObject {
			// yaml 未配置四项时 bool 全 false：使用 P0/P1/P2 默认
			sessionCam = true
			snapAudioEnd = true
			snapObject = true
		}
	}
	return VisionPublicConfig{
		SessionCamera:          sessionCam,
		SnapshotOnSpeechStart:  snapSpeech,
		SnapshotOnAudioEnd:     snapAudioEnd,
		SnapshotOnObjectIntent: snapObject,
	}
}

func (v *VisionConfig) applyDefaults() {
	if v.Model == "" {
		v.Model = "qwen-vl-plus"
	}
	if v.TimeoutMS == 0 {
		v.TimeoutMS = 5000
	}
	if v.DefaultFocus == "" {
		v.DefaultFocus = "owner_face"
	}
	if v.MinExpressionConfidence == 0 {
		v.MinExpressionConfidence = 0.6
	}
	if v.EarlyAnimationMinConf == 0 {
		v.EarlyAnimationMinConf = 0.65
	}
	if v.ClassifyTimeoutMS == 0 {
		v.ClassifyTimeoutMS = 800
	}
	if len(v.ObjectTriggerKeywords) == 0 {
		v.ObjectTriggerKeywords = []string{
			"这是什么", "看看这个", "帮我看看", "认认这个", "是什么东西", "这啥", "what is this",
		}
	}
	if len(v.SceneTriggerKeywords) == 0 {
		v.SceneTriggerKeywords = []string{
			"你看外面", "你看窗外", "看看外面", "你看房间", "看看环境", "你看周围", "你看一下外面",
		}
	}
}

type ToolsConfig struct {
	Enabled               bool   `yaml:"enabled"`
	Mode                  string `yaml:"mode"`
	MinRapportForSuggest  int    `yaml:"min_rapport_for_suggest"`
	MinTrustForAutoCreate int    `yaml:"min_trust_for_auto_create"`
	ReminderTickSeconds   int    `yaml:"reminder_tick_seconds"`
	MaxPendingReminders   int    `yaml:"max_pending_reminders"`
	ToolTurnMaxTokens     int    `yaml:"tool_turn_max_tokens"`
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

var loadedConfigPath string

// LoadedPath returns the absolute path of the config file used by Load().
func LoadedPath() string {
	return loadedConfigPath
}

func Load() (*Config, error) {
	path, err := findConfigPath()
	if err != nil {
		return nil, err
	}
	loadedConfigPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.loadDataFiles(filepath.Dir(path)); err != nil {
		return nil, err
	}
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
	if c.AI.EnableSearch && c.AI.SearchStrategy == "" {
		c.AI.SearchStrategy = "turbo"
	}
	if c.Client.APIBase == "" {
		c.Client.APIBase = fmt.Sprintf("http://localhost:%d", c.Server.Port)
	}
	if c.Log.Dir == "" {
		c.Log.Dir = "logs"
	}
	c.Realtime.applyDefaults()
	c.Companion.applyDefaults()
	c.Growth.applyDefaults()
	c.Tools.applyDefaults()
	c.Wellness.applyDefaults()
	c.Emotion.applyDefaults()
	c.Vision.applyDefaults()
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

func (c *WellnessConfig) applyDefaults() {
	if c.TickMinutes == 0 {
		c.TickMinutes = 10
	}
	if c.MaxDailyNudges == 0 {
		c.MaxDailyNudges = 2
	}
	if c.ActivityIdleBreakMinutes == 0 {
		c.ActivityIdleBreakMinutes = 5
	}
	if c.DrinkActiveMinutes == 0 {
		c.DrinkActiveMinutes = 90
	}
	if c.RestActiveMinutes == 0 {
		c.RestActiveMinutes = 120
	}
	if c.OverworkActiveMinutes == 0 {
		c.OverworkActiveMinutes = 180
	}
	if c.EveningRestHour == 0 {
		c.EveningRestHour = 22
	}
}

func (c *ToolsConfig) applyDefaults() {
	if c.Mode == "" {
		c.Mode = "tool_calling"
	}
	if c.MinRapportForSuggest == 0 {
		c.MinRapportForSuggest = 60
	}
	if c.MinTrustForAutoCreate == 0 {
		c.MinTrustForAutoCreate = 30
	}
	if c.ReminderTickSeconds == 0 {
		c.ReminderTickSeconds = 5
	}
	if c.MaxPendingReminders == 0 {
		c.MaxPendingReminders = 50
	}
	if c.ToolTurnMaxTokens == 0 {
		c.ToolTurnMaxTokens = 256
	}
}

func (r *RealtimeConfig) applyDefaults() {
	if r.STTMode == "" {
		r.STTMode = "auto"
	}
	if r.VAD.SilenceMS == 0 {
		r.VAD.SilenceMS = 1200
	}
	if r.VAD.MinSpeechMS == 0 {
		r.VAD.MinSpeechMS = 300
	}
	if r.VAD.Model == "" {
		r.VAD.Model = "energy"
	}
	if r.VAD.EnergyPeak == 0 {
		r.VAD.EnergyPeak = 0.05
	}
	if r.VAD.TailSpeechPeak == 0 {
		r.VAD.TailSpeechPeak = 0.015
	}
	if r.VAD.PlaybackPeak == 0 {
		r.VAD.PlaybackPeak = 0.10
	}
	if r.VAD.Silero.PositiveThreshold == 0 {
		r.VAD.Silero.PositiveThreshold = 0.5
	}
	if r.VAD.Silero.NegativeThreshold == 0 {
		r.VAD.Silero.NegativeThreshold = 0.35
	}
	if r.VAD.Silero.RedemptionMS == 0 {
		r.VAD.Silero.RedemptionMS = 800
	}
	if r.VAD.Silero.MinSpeechMS == 0 {
		r.VAD.Silero.MinSpeechMS = 350
	}
	if r.VAD.Silero.PreSpeechPadMS == 0 {
		r.VAD.Silero.PreSpeechPadMS = 300
	}
	if r.BargeIn.EchoGuardMS == 0 {
		r.BargeIn.EchoGuardMS = 1800
	}
	if r.BargeIn.EchoGuardMSAEC == 0 {
		r.BargeIn.EchoGuardMSAEC = 200
	}
	if r.BargeIn.PeakThreshold == 0 {
		r.BargeIn.PeakThreshold = 0.06
	}
	if r.BargeIn.BargeInMS == 0 {
		r.BargeIn.BargeInMS = 800
	}
	if r.Voiceprint.Threshold == 0 {
		r.Voiceprint.Threshold = 0.38
		r.Voiceprint.Required = true
	}
	if r.Voiceprint.VerifyWindowSec == 0 {
		r.Voiceprint.VerifyWindowSec = 4.0
	}
	if r.Voiceprint.WakeProbeSec == 0 {
		r.Voiceprint.WakeProbeSec = 1.0
	}
	if r.Voiceprint.StreamCheckIntervalMS == 0 {
		r.Voiceprint.StreamCheckIntervalMS = 500
	}
	if r.Voiceprint.RejectStreak == 0 {
		r.Voiceprint.RejectStreak = 3
	}
	if r.Voiceprint.OwnerRecentMS == 0 {
		r.Voiceprint.OwnerRecentMS = 8000
	}
	if r.Voiceprint.NonOwnerReplyCooldownMS == 0 {
		r.Voiceprint.NonOwnerReplyCooldownMS = 12000
	}
	if r.Faceprint.MatchThreshold == 0 {
		r.Faceprint.MatchThreshold = 0.42
	}
	if r.Faceprint.GrayZoneLow == 0 {
		r.Faceprint.GrayZoneLow = 0.28
	}
	if r.Faceprint.CheckIntervalMS == 0 {
		r.Faceprint.CheckIntervalMS = 2000
	}
	if r.Faceprint.OwnerRecentMS == 0 {
		r.Faceprint.OwnerRecentMS = r.Voiceprint.OwnerRecentMS
		if r.Faceprint.OwnerRecentMS == 0 {
			r.Faceprint.OwnerRecentMS = 8000
		}
	}
	if r.Faceprint.EnrollSamples == 0 {
		r.Faceprint.EnrollSamples = 3
	}
	if r.Presence.AmbientIntervalMS == 0 {
		r.Presence.AmbientIntervalMS = 2000
		r.Presence.Enabled = true
	}
	if r.Presence.AwayTimeoutSec == 0 {
		r.Presence.AwayTimeoutSec = 180
	}
	if r.Presence.SpeechThreshold == 0 {
		r.Presence.SpeechThreshold = 0.30
	}
	if r.Presence.AmbientEnergyFloor == 0 {
		r.Presence.AmbientEnergyFloor = 0.012
	}
	if r.Presence.OwnerPresenceTTLSec == 0 {
		r.Presence.OwnerPresenceTTLSec = 30
	}
	if r.TopicAnchor.StickyTurns == 0 {
		r.TopicAnchor.StickyTurns = 3
	}
	if r.ThinkingFiller.Enabled {
		if r.ThinkingFiller.ThresholdMS == 0 {
			r.ThinkingFiller.ThresholdMS = 800
		}
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
	if r.TTS.Transport == "" {
		r.TTS.Transport = "opus"
	}
	if r.TTS.Opus.SampleRate == 0 {
		r.TTS.Opus.SampleRate = 48000
	}
	if r.TTS.Opus.Bitrate == 0 {
		r.TTS.Opus.Bitrate = 24000
	}
	if r.TTS.Opus.FrameMS == 0 {
		r.TTS.Opus.FrameMS = 20
	}
	if r.TTS.Opus.Application == "" {
		r.TTS.Opus.Application = "voip"
	}
	r.Pipeline.applyDefaults()
	if r.Gate.Model == "" {
		r.Gate.Model = "qwen-turbo"
	}
	if r.Gate.TimeoutMS == 0 {
		r.Gate.TimeoutMS = 800
	}
	if r.Gate.MaxChars == 0 {
		r.Gate.MaxChars = 200
	}
	if r.Gate.MaxTokens == 0 {
		r.Gate.MaxTokens = 20
	}
}

func (p *RealtimePipeline) applyDefaults() {
	if p.TTSMinChars == 0 {
		p.TTSMinChars = 8
	}
	if p.TTSFirstMinChars == 0 {
		p.TTSFirstMinChars = 3
	}
	if p.TTSWeakPunctMinChars == 0 {
		p.TTSWeakPunctMinChars = 8
	}
	if p.TTSForceFlushChars == 0 {
		p.TTSForceFlushChars = 24
	}
	if p.TTSPunctuation == "" {
		p.TTSPunctuation = "。！？，、~.!?,;"
	}
	if p.TTSStrongPunctuation == "" {
		p.TTSStrongPunctuation = "。！？~!?.\n"
	}
	if p.TTSWeakPunctuation == "" {
		p.TTSWeakPunctuation = "，、,"
	}
}

// PublicClient returns client-safe realtime tuning knobs.
func (r RealtimeConfig) PublicClient() RealtimePublicConfig {
	out := RealtimePublicConfig{
		STTMode:      r.STTMode,
		SpeechLocale: "zh-CN",
		TTSTransport: r.TTS.Transport,
	}
	out.VAD.SilenceMS = r.VAD.SilenceMS
	out.VAD.MinSpeechMS = r.VAD.MinSpeechMS
	out.VAD.EndpointingEnabled = r.VAD.EndpointingEnabled
	out.VAD.EnergyPeak = r.VAD.EnergyPeak
	out.VAD.TailSpeechPeak = r.VAD.TailSpeechPeak
	out.VAD.PlaybackPeak = r.VAD.PlaybackPeak
	wakePeak := r.VAD.WakePeak
	if wakePeak == 0 {
		wakePeak = r.BargeIn.PeakThreshold
	}
	out.VAD.WakePeak = wakePeak
	out.VAD.Silero.PositiveThreshold = r.VAD.Silero.PositiveThreshold
	out.VAD.Silero.NegativeThreshold = r.VAD.Silero.NegativeThreshold
	out.VAD.Silero.RedemptionMS = r.VAD.Silero.RedemptionMS
	out.VAD.Silero.MinSpeechMS = r.VAD.Silero.MinSpeechMS
	out.VAD.Silero.PreSpeechPadMS = r.VAD.Silero.PreSpeechPadMS
	out.BargeIn.EchoGuardMS = r.BargeIn.EchoGuardMS
	out.BargeIn.PeakThreshold = r.BargeIn.PeakThreshold
	out.BargeIn.BargeInMS = r.BargeIn.BargeInMS
	out.Voiceprint.Required = r.Voiceprint.Required
	out.Voiceprint.Threshold = r.Voiceprint.Threshold
	out.Voiceprint.VerifyWindowSec = r.Voiceprint.VerifyWindowSec
	out.Voiceprint.WakeProbeSec = r.Voiceprint.WakeProbeSec
	out.Voiceprint.StreamCheckIntervalMS = r.Voiceprint.StreamCheckIntervalMS
	out.Voiceprint.RejectStreak = r.Voiceprint.RejectStreak
	out.Voiceprint.OwnerRecentMS = r.Voiceprint.OwnerRecentMS
	out.Voiceprint.NonOwnerReplyCooldownMS = r.Voiceprint.NonOwnerReplyCooldownMS
	out.Faceprint.Enabled = r.Faceprint.Enabled
	out.Faceprint.Required = r.Faceprint.Required
	out.Faceprint.MatchThreshold = r.Faceprint.MatchThreshold
	out.Faceprint.GrayZoneLow = r.Faceprint.GrayZoneLow
	out.Faceprint.ProbeOnSpeechStart = r.Faceprint.ProbeOnSpeechStart
	out.Faceprint.CheckIntervalMS = r.Faceprint.CheckIntervalMS
	out.Faceprint.OwnerRecentMS = r.Faceprint.OwnerRecentMS
	out.Faceprint.EnrollSamples = r.Faceprint.EnrollSamples
	out.Presence.Enabled = r.Presence.Enabled
	out.Presence.AmbientIntervalMS = r.Presence.AmbientIntervalMS
	out.Presence.AwayTimeoutSec = r.Presence.AwayTimeoutSec
	out.Presence.SpeechThreshold = r.Presence.SpeechThreshold
	out.Presence.AmbientEnergyFloor = r.Presence.AmbientEnergyFloor
	out.Presence.OwnerPresenceTTLSec = r.Presence.OwnerPresenceTTLSec
	return out
}

func findConfigPath() (string, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("CONFIG_PATH not found: %s", p)
	}

	candidates := []string{
		filepath.Join("config", "config.yaml"),
		filepath.Join("..", "config", "config.yaml"),
		filepath.Join("..", "..", "config", "config.yaml"),
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
	return "", fmt.Errorf("config not found (set CONFIG_PATH or use config/config.yaml)")
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
