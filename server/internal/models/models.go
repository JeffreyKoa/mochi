package models

import (
	"time"
)

type User struct {
	ID                 uint64    `gorm:"primaryKey" json:"id"`
	Email              string    `gorm:"uniqueIndex;size:255" json:"email"`
	Password           string    `gorm:"size:255" json:"-"`
	ProactiveEnabled     bool `gorm:"default:true" json:"proactive_enabled"`
	PresenceChatEnabled  bool `gorm:"default:true" json:"presence_chat_enabled"`
	QuietHoursStart    int       `gorm:"default:23" json:"quiet_hours_start"`
	QuietHoursEnd      int       `gorm:"default:8" json:"quiet_hours_end"`
	MorningGreeting    bool      `gorm:"default:true" json:"morning_greeting"`
	ReminderVoice      bool      `gorm:"default:true" json:"reminder_voice"`
	FollowUpEnabled    bool      `gorm:"default:true" json:"follow_up_enabled"`
	VoiceReplyDefault  bool      `gorm:"default:true" json:"voice_reply_default"`
	SttMode                string `gorm:"size:16;default:auto" json:"stt_mode"`
	WellnessNudgesEnabled  bool   `gorm:"default:true" json:"wellness_nudges_enabled"`
	WellnessDrink          bool   `gorm:"default:true" json:"wellness_drink"`
	WellnessMeal           bool   `gorm:"default:true" json:"wellness_meal"`
	WellnessRest           bool   `gorm:"default:true" json:"wellness_rest"`
	LunchHour              int    `gorm:"default:12" json:"lunch_hour"`
	DinnerHour             int    `gorm:"default:18" json:"dinner_hour"`
	WellnessDailyMax       int    `gorm:"default:2" json:"wellness_daily_max"`
	LearningPrefsJSON      []byte `gorm:"type:json" json:"learning_prefs,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Pet                *Pet      `gorm:"foreignKey:UserID" json:"pet,omitempty"`
}

type LearningPrefs struct {
	LearningTopics       []string `json:"learning_topics,omitempty"`
	EnglishLevel         string   `json:"english_level,omitempty"`
	StudyPaceMinutes     int      `json:"study_pace_minutes,omitempty"`
	NoUnsolicitedAdvice  bool     `json:"no_unsolicited_advice"`
}

type Pet struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	UserID          uint64         `gorm:"uniqueIndex" json:"user_id"`
	Name            string         `gorm:"size:64;default:Mochi" json:"name"`
	PersonalityJSON []byte         `gorm:"type:json" json:"personality"`
	SKUId           string         `gorm:"column:sku_id;size:64" json:"sku_id"`
	Species         string         `gorm:"size:16;default:cat" json:"species"`
	Breed           string         `gorm:"size:32" json:"breed"`
	Gender          string         `gorm:"size:8;default:female" json:"gender"`
	BornAt          time.Time      `json:"born_at"`
	MaxAgeYears     float32        `gorm:"default:18" json:"max_age_years"`
	LifeStage       string         `gorm:"size:16;default:newborn" json:"life_stage"`
	IsAlive         bool           `gorm:"default:true" json:"is_alive"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LifeState       *LifeState     `gorm:"foreignKey:PetID" json:"life_state,omitempty"`
}

type Personality struct {
	Traits      string   `json:"traits"`
	SpeechStyle string   `json:"speech_style"`
	StyleNotes  []string `json:"style_notes,omitempty"`

	// 0-100 Personality Vector dimensions
	Warmth     int `json:"warmth,omitempty"`     // 温暖度：控制安慰方式与关心程度
	Humor      int `json:"humor,omitempty"`      // 幽默度：控制打趣与彩蛋内容
	Strictness int `json:"strictness,omitempty"` // 严格度：控制对任务/拖延的监督力度
	Curiosity  int `json:"curiosity,omitempty"`  // 好奇心：控制反问与引导探索倾向
	Confidence int `json:"confidence,omitempty"` // 自信度：控制语气的果断/谦虚度
	Empathy    int `json:"empathy,omitempty"`    // 共情力：对负面情绪的容受与理解
	Energy     int `json:"energy,omitempty"`     // 活跃度：控制主动说话的频率（低/中/高）
	Logic      int `json:"logic,omitempty"`      // 理性/智慧：回答的深度与结构条理性
	Sarcasm    int `json:"sarcasm,omitempty"`    // 毒舌/傲娇度：反讽与口是心非程度
}

type StyleConfig struct {
	SentenceLength string   `json:"sentence_length"` // "short" | "medium" | "long"
	EmojiRate      float64  `json:"emoji_rate"`      // 0.0 ~ 1.0
	Punctuation    string   `json:"punctuation"`     // "cute" | "standard" | "minimal"
	Nickname       string   `json:"nickname"`        // "主人" | 自定义称呼
	HumorLevel     int      `json:"humor_level"`     // 幽默感强度 0-100
	ToneModifiers  []string `json:"tone_modifiers"`   // 语气词提示如 ["温柔", "傲娇", "稳重"]
}

type ChatMessage struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	PetID     uint64    `gorm:"index" json:"pet_id"`
	Role      string    `gorm:"type:enum('user','assistant');size:16" json:"role"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Memory struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	PetID      uint64    `gorm:"index" json:"pet_id"`
	Type       string    `gorm:"type:varchar(16);default:long" json:"type"`
	Content    string    `gorm:"size:1024" json:"content"`
	Importance float32   `gorm:"default:0.5" json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
}

type BondProfile struct {
	PetID        uint64    `gorm:"primaryKey" json:"pet_id"`
	RapportLevel uint8     `gorm:"default:20" json:"rapport_level"`
	TrustLevel   uint8     `gorm:"default:15" json:"trust_level"`
	SharedTopics []byte    `gorm:"type:json" json:"shared_topics"`
	Nicknames    []byte    `gorm:"type:json" json:"nicknames"`
	InsideJokes  []byte    `gorm:"type:json" json:"inside_jokes"`
	LastMoodTag  string    `gorm:"size:32" json:"last_mood_tag"`
	LastIntent   string    `gorm:"size:32" json:"last_intent"`
	LastMoodAt   time.Time `json:"last_mood_at"`
	TotalTurns   int       `gorm:"default:0" json:"total_turns"`
	LastChatDay  string    `gorm:"size:10" json:"last_chat_day"` // YYYY-MM-DD
	StreakDays   int       `gorm:"default:0" json:"streak_days"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BondNicknames struct {
	UserCallsPet string `json:"user_calls_pet"`
	PetCallsUser string `json:"pet_calls_user"`
}

type BondInsideJoke struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type LifeState struct {
	PetID           uint64    `gorm:"primaryKey" json:"pet_id"`
	Mood            uint8     `gorm:"default:70" json:"mood"`
	EmotionState    string    `gorm:"type:varchar(16);default:calm" json:"emotion_state"` // 心境状态 FSM: calm|happy|worried|sad|excited
	Love            uint8     `gorm:"default:60" json:"love"`
	Hungry          uint8     `gorm:"default:30" json:"hungry"`
	Energy          uint8     `gorm:"default:80" json:"energy"`
	Health          uint8     `gorm:"default:90" json:"health"`
	Sleep           uint8     `gorm:"default:20" json:"sleep"`
	Curiosity       uint8     `gorm:"default:50" json:"curiosity"`
	Knowledge       uint8     `gorm:"default:40" json:"knowledge"`
	LastInteraction time.Time `json:"last_interaction"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserBrief struct {
	PetID        uint64    `gorm:"primaryKey" json:"pet_id"`
	CompiledText string    `gorm:"size:1400" json:"compiled_text"`
	CompiledAt   time.Time `json:"compiled_at"`
	CharBudget   uint16    `gorm:"default:1400" json:"char_budget"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UserBriefEntry struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	PetID      uint64    `gorm:"index" json:"pet_id"`
	Category   string    `gorm:"size:16" json:"category"`
	Content    string    `gorm:"size:256" json:"content"`
	Importance float32   `json:"importance"`
	Source     string    `gorm:"size:16" json:"source"`
	Status     string    `gorm:"size:16;default:approved" json:"status"` // approved | pending | rejected
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PetSKU struct {
	SKUId            string    `gorm:"column:sku_id;primaryKey;size:64" json:"sku_id"`
	Name             string    `gorm:"size:64" json:"name"`
	Species          string    `gorm:"size:16;default:cat" json:"species"`
	Breed            string    `gorm:"size:32" json:"breed"`
	BreedName        string    `gorm:"size:64" json:"breed_name"`
	Tier             string    `gorm:"size:16;default:standard" json:"tier"`
	MaxAgeYears      float32   `gorm:"default:18" json:"max_age_years"`
	PriceCNY         int       `gorm:"default:0" json:"price_cny"`
	Tagline          string    `gorm:"size:128" json:"tagline"`
	SkinJSON         []byte    `gorm:"type:json" json:"skin"`
	PersonalityJSON  []byte    `gorm:"type:json" json:"personality_preset"`
	SortOrder        int       `gorm:"default:0" json:"sort_order"`
	Enabled          bool      `gorm:"default:true" json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (PetSKU) TableName() string { return "pet_skus" }

type PetOrder struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	UserID           uint64    `gorm:"index" json:"user_id"`
	SKUId            string    `gorm:"column:sku_id;size:64" json:"sku_id"`
	Status           string    `gorm:"size:16;default:pending" json:"status"`
	PersonalityJSON  []byte    `gorm:"type:json" json:"personality,omitempty"`
	PetID            *uint64   `json:"pet_id,omitempty"`
	PaidAt           *time.Time `json:"paid_at,omitempty"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (PetOrder) TableName() string { return "pet_orders" }

type Reminder struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	PetID      uint64     `gorm:"index" json:"pet_id"`
	UserID     uint64     `gorm:"index" json:"user_id"`
	Title      string     `gorm:"size:256" json:"title"`
	FireAt     time.Time  `json:"fire_at"`
	RepeatRule string     `gorm:"size:32" json:"repeat_rule,omitempty"`
	Status     string     `gorm:"size:16;default:pending" json:"status"`
	Source     string     `gorm:"size:16;default:chat" json:"source"`
	SourceMsg  string     `gorm:"size:512" json:"source_msg,omitempty"`
	FiredAt    *time.Time `json:"fired_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type WellnessNudgeLog struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	PetID     uint64    `gorm:"index" json:"pet_id"`
	UserID    uint64    `gorm:"index" json:"user_id"`
	Type      string    `gorm:"size:32" json:"type"`
	Message   string    `gorm:"size:512" json:"message"`
	SentAt    time.Time `json:"sent_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Todo struct {
	ID        uint64     `gorm:"primaryKey" json:"id"`
	PetID     uint64     `gorm:"index" json:"pet_id"`
	UserID    uint64     `gorm:"index" json:"user_id"`
	Title     string     `gorm:"size:256" json:"title"`
	DueAt     *time.Time `json:"due_at,omitempty"`
	Done      bool       `gorm:"default:false" json:"done"`
	SortOrder int        `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Voiceprint stores the owner's averaged speaker embedding for client-side verification.
type Voiceprint struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"uniqueIndex" json:"user_id"`
	Embedding string    `gorm:"type:text" json:"-"`
	Dim       int       `json:"dim"`
	Samples   int       `json:"samples"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Faceprint stores the owner's averaged face embedding for client-side verification (P2).
type Faceprint struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"uniqueIndex" json:"user_id"`
	Embedding string    `gorm:"type:text" json:"-"`
	Dim       int       `json:"dim"`
	Samples   int       `json:"samples"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
