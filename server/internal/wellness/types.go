package wellness

import "time"

type NudgeKind string

const (
	NudgeDrink    NudgeKind = "wellness_drink"
	NudgeMeal     NudgeKind = "wellness_meal"
	NudgeRest     NudgeKind = "wellness_rest"
	NudgeOverwork NudgeKind = "wellness_overwork"
)

type OwnerActivity struct {
	IdleSeconds               int       `json:"idle_seconds"`
	ContinuousActiveMinutes   int       `json:"continuous_active_minutes"`
	SessionActiveMinutesToday int       `json:"session_active_minutes_today"`
	ActiveApp                 string    `json:"active_app"`
	SoundPresence             string    `json:"sound_presence,omitempty"`
	LastHumanVoiceSec         *int      `json:"last_human_voice_sec,omitempty"`
	LastOwnerVoiceSec         *int      `json:"last_owner_voice_sec,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type HeartbeatInput struct {
	IdleSeconds               int    `json:"idle_seconds"`
	ContinuousActiveMinutes   int    `json:"continuous_active_minutes"`
	SessionActiveMinutesToday int    `json:"session_active_minutes_today"`
	ActiveApp                 string `json:"active_app"`
	SoundPresence             string `json:"sound_presence,omitempty"`
	LastHumanVoiceSec         *int   `json:"last_human_voice_sec,omitempty"`
	LastOwnerVoiceSec         *int   `json:"last_owner_voice_sec,omitempty"`
}
