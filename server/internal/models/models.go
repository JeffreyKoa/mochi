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
	Type       string    `gorm:"type:enum('long','event','relation');default:long" json:"type"`
	Content    string    `gorm:"size:1024" json:"content"`
	Importance float32   `gorm:"default:0.5" json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
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
