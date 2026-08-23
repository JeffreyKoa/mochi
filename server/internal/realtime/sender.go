package realtime

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"sync"
)

type WSMessage struct {
	IsBinary bool
	Data     []byte
}

type Sender interface {
	Send(msgType string, data any) error
	SendTTSAudioBinary(audio []byte, format string, seq, ttsEpoch int64) error
	SendAnimation(state SessionState)
}

type connSender struct {
	mu   sync.Mutex
	send func(WSMessage) error
}

func (s *connSender) Send(msgType string, data any) error {
	b, err := marshalMsg(msgType, data, 0)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.send(WSMessage{IsBinary: false, Data: b})
}

func (s *connSender) SendTTSAudioBinary(audio []byte, format string, seq, ttsEpoch int64) error {
	// 18-byte header: type(1) + format(1) + seq(8) + tts_epoch(8) + audio
	buf := make([]byte, 18+len(audio))
	buf[0] = 0x01 // MsgType: TTS Audio Binary
	formatByte := byte(0x01) // mp3
	switch format {
	case "pcm":
		formatByte = 0x02
	case "opus":
		formatByte = 0x03
	case "wav":
		formatByte = 0x04
	}
	buf[1] = formatByte
	binary.BigEndian.PutUint64(buf[2:10], uint64(seq))
	binary.BigEndian.PutUint64(buf[10:18], uint64(ttsEpoch))
	copy(buf[18:], audio)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.send(WSMessage{IsBinary: true, Data: buf})
}

func (s *connSender) SendAnimation(state SessionState) {
	_ = s.Send(MsgAnimation, AnimationState{State: string(state)})
}

func decodePCM(b64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64)
}

func parseClientMsg(raw []byte) (string, json.RawMessage, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", nil, err
	}
	return env.Type, env.Data, nil
}
