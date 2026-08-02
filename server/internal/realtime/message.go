package realtime

import "encoding/json"

// Client → Server message types
const (
	MsgAudio      = "audio"
	MsgAudioStart = "audio_start"
	MsgAudioEnd   = "audio_end"
	MsgTextInput  = "text_input"
	MsgHeartbeat    = "heartbeat"
	MsgInterrupt    = "interrupt"
	MsgPrewarm      = "prewarm"
	MsgPlaybackMark = "playback_mark"
	MsgClientCaps   = "client_caps"
	MsgVisionFrame     = "vision_frame"
	MsgNonOwnerTurn    = "non_owner_turn"
	MsgUtteranceCancel = "utterance_cancel"
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
	MsgTTSSegmentDone = "tts_segment_done"
	MsgTTSDone      = "tts_done"
	MsgInterrupted  = "interrupted"
	MsgTurnAck      = "turn_ack"
	MsgAnimation    = "animation"
	MsgError        = "error"
	MsgAck          = "ack"
	MsgTurnMetrics       = "turn_metrics"
	MsgProactiveMessage  = "proactive_message"
	MsgTTSStreamStart    = "tts_stream_start"
	MsgBargeInConfig     = "barge_in_config"
)

type TTSStreamStart struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	FrameMS    int    `json:"frame_ms"`
	Bitrate    int    `json:"bitrate"`
}

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

// VisionFrameIn 客户端上传的单帧 JPEG（语音 turn 前抓拍）。
type VisionFrameIn struct {
	JPEG   string `json:"jpeg"` // base64 encoded JPEG，日志禁止打印
	Seq    int64  `json:"seq,omitempty"`
	Reason string `json:"reason,omitempty"` // speech_start | audio_end
}

type TextInput struct {
	Text       string `json:"text"`
	VoiceReply bool   `json:"voice_reply,omitempty"`
}

// ClientCaps advertises playback capabilities so the server can pick a compatible TTS transport.
type ClientCaps struct {
	OpusDecode bool `json:"opus_decode"`
	AecEnabled bool `json:"aec_enabled"` // WebView getUserMedia echoCancellation 实际生效
}

// BargeInConfig 服务端下发的打断参数（AEC 握手后 echo_guard_ms 可缩短）。
type BargeInConfig struct {
	EchoGuardMS   int     `json:"echo_guard_ms"`
	PeakThreshold float64 `json:"peak_threshold"`
	BargeInMS     int     `json:"barge_in_ms"`
	AecEnabled    bool    `json:"aec_enabled"`
}

type VADEvent struct {
	Event string `json:"event"` // speech_start | speech_end
}

type ASRText struct {
	Text        string `json:"text"`
	SentenceEnd bool   `json:"sentence_end,omitempty"`
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

type TurnMetrics struct {
	AudioEndMS          int64 `json:"audio_end_ms"`
	ASRFinalMS          int64 `json:"asr_final_ms"`
	VisionMS            int64 `json:"vision_ms"`
	PerceiveParallelMS  int64 `json:"perceive_parallel_ms"`
	LLMFirstTokenMS     int64 `json:"llm_first_token_ms"`
	LLMFirstSentenceMS  int64 `json:"llm_first_sentence_ms"`
	TTSFirstByteMS      int64 `json:"tts_first_byte_ms"`
	PlaybackStartMS     int64 `json:"playback_start_ms"`
	FillerPlayedMS      int64 `json:"filler_played_ms"`
}

type PlaybackMark struct {
	AtMS int64 `json:"at_ms"`
}

type ProactiveMessage struct {
	Message    string `json:"message"`
	Animation  string `json:"animation"`
	ReminderID uint64 `json:"reminder_id,omitempty"`
}

func marshalMsg(msgType string, data any, seq int64) ([]byte, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	env := Envelope{Type: msgType, Data: raw, Seq: seq}
	return json.Marshal(env)
}
