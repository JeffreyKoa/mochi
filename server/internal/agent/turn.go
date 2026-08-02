package agent

import (
	"time"

	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/vision"
	"github.com/mochi-ai/server/pkg/ai"
)

// TopicAnchorInput 会话级话题锚点（P1），注入 LLM L3。
type TopicAnchorInput struct {
	CurrentTopic string `json:"current_topic,omitempty"`
	OpenQuestion string `json:"open_question,omitempty"`
}

// TurnInput represents the input parameters for a single Agent interaction turn.
type TurnInput struct {
	UserID          uint64                 `json:"user_id"`
	PetID           uint64                 `json:"pet_id"`
	Message         string                 `json:"message"`          // User text message (empty for proactive triggers)
	TriggerType     string                 `json:"trigger_type"`     // "user_chat" | "user_voice" | "system_proactive"
	ActivityContext map[string]interface{} `json:"activity_context"` // Client activity context (active application, idle state, etc.)
	AcousticHint    emotion.AcousticHint   `json:"acoustic_hint,omitempty"` // 语音 turn 的声学情绪（Phase 2）
	VisualHint      vision.Hint            `json:"visual_hint,omitempty"`   // 语音 turn 的视觉感知（Phase 1）
	PipelinePerception *emotion.PerceptionState `json:"pipeline_perception,omitempty"` // V3c：Pipeline 融合唯一源
	TopicAnchor     TopicAnchorInput       `json:"topic_anchor,omitempty"`
}

// TurnOutput represents the output of a single Agent interaction turn, supporting streaming.
type TurnOutput struct {
	ReplyStream <-chan ai.ChatChunk
	Trace       *TurnTrace
}

// PerceptionResult represents the structured output of the Perceive step.
type PerceptionResult struct {
	UserMood     string  `json:"user_mood"`
	Intensity    float64 `json:"intensity"`
	Topic        string  `json:"topic"`
	NeedsEmpathy bool    `json:"needs_empathy"`
	Intent       string  `json:"intent"`
}

// PersonalityDecision captures the DecideStyle step output (Phase 1 pass-through).
type PersonalityDecision struct {
	Strategy string `json:"strategy"`
	Notes    string `json:"notes,omitempty"`
}

// StepTimings records per-step latency inside a Turn (milliseconds).
type StepTimings struct {
	PerceiveMs    int64 `json:"perceive_ms,omitempty"`
	RecallMs      int64 `json:"recall_ms,omitempty"`
	DecideMs      int64 `json:"decide_ms,omitempty"`
	BuildPromptMs int64 `json:"build_prompt_ms,omitempty"`
	InvokeLLMMs   int64 `json:"invoke_llm_ms,omitempty"`
	PostTurnMs    int64 `json:"post_turn_ms,omitempty"`
}

// TurnTrace captures timing and diagnostic logs for observability of the Turn execution.
type TurnTrace struct {
	PetID               uint64              `json:"pet_id"`
	UserID              uint64              `json:"user_id"`
	InputMessage        string              `json:"input_message"`
	TriggerType         string              `json:"trigger_type"`
	ActivityContext     map[string]interface{} `json:"activity_context,omitempty"`
	StartTime           time.Time           `json:"start_time"`
	DurationMs          int64               `json:"duration_ms"`
	StepTimings         StepTimings         `json:"step_timings"`
	Perception          PerceptionResult    `json:"perception"`
	PersonalityDecision PersonalityDecision `json:"personality_decision"`
	MemoryHitCount      int                 `json:"memory_hit_count"`
	SelectedModel       string              `json:"selected_model"`
	LLMError            string              `json:"llm_error,omitempty"`
}
