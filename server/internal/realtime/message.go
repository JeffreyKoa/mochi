package realtime

import "encoding/json"

// Client → Server message types
const (
	MsgAudio      = "audio"
	MsgAudioStart = "audio_start"
	MsgAudioEnd   = "audio_end"
	MsgTextInput  = "text_input"
	MsgHeartbeat  = "heartbeat"
	MsgInterrupt  = "interrupt"
)

// Server → Client message types
const (
	MsgSessionStart = "session_start"
	MsgVAD          = "vad"
	MsgASRPartial   = "asr_partial"
	MsgASRFinal     = "asr_final"
	MsgLLMToken     = "llm_token"
	MsgLLMDone      = "llm_done"
	MsgTTSAudio     = "tts_audio"
	MsgTTSDone      = "tts_done"
	MsgInterrupted  = "interrupted"
	MsgTurnAck      = "turn_ack"
	MsgAnimation    = "animation"
	MsgError        = "error"
	MsgAck          = "ack"
)

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
	Seq  int64           `json:"seq,omitempty"`
	Ts   int64           `json:"ts,omitempty"`
}

type AudioIn struct {
	PCM string `json:"pcm"` // base64 encoded PCM int16 LE
	Seq int64  `json:"seq"`
}

type TextInput struct {
	Text string `json:"text"`
}

type VADEvent struct {
	Event string `json:"event"` // speech_start | speech_end
}

type ASRText struct {
	Text string `json:"text"`
}

type LLMToken struct {
	Token string `json:"token"`
}

type LLMDone struct {
	Text string `json:"text"`
}

type TTSAudio struct {
	PCM    string `json:"pcm"`    // base64 encoded audio
	Format string `json:"format"` // mp3 | pcm
	Seq    int64  `json:"seq"`
}

type AnimationState struct {
	State string `json:"state"` // idle | listening | thinking | speaking
}

type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SessionStart struct {
	SessionID string `json:"session_id"`
}

type AckData struct {
	Seq int64 `json:"seq"`
}

func marshalMsg(msgType string, data any, seq int64) ([]byte, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	env := Envelope{Type: msgType, Data: raw, Seq: seq}
	return json.Marshal(env)
}
