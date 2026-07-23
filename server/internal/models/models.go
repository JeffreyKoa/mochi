package models

import (
	"time"
)

type User struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"uniqueIndex;size:255" json:"email"`
	Password  string    `gorm:"size:255" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Pet       *Pet      `gorm:"foreignKey:UserID" json:"pet,omitempty"`
}

type Pet struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	UserID          uint64         `gorm:"uniqueIndex" json:"user_id"`
	Name            string         `gorm:"size:64;default:Mochi" json:"name"`
	PersonalityJSON []byte `gorm:"type:json" json:"personality"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LifeState       *LifeState     `gorm:"foreignKey:PetID" json:"life_state,omitempty"`
}

type Personality struct {
	Traits      string `json:"traits"`
	SpeechStyle string `json:"speech_style"`
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
	Love            uint8     `gorm:"default:60" json:"love"`
	Hungry          uint8     `gorm:"default:30" json:"hungry"`
	Energy          uint8     `gorm:"default:80" json:"energy"`
	LastInteraction time.Time `json:"last_interaction"`
	UpdatedAt       time.Time `json:"updated_at"`
}
