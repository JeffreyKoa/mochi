package realtime

import (
	"encoding/base64"
	"encoding/json"
	"sync"
)

type Sender interface {
	Send(msgType string, data any) error
	SendAnimation(state SessionState)
}

type connSender struct {
	mu   sync.Mutex
	send func([]byte) error
}

func (s *connSender) Send(msgType string, data any) error {
	b, err := marshalMsg(msgType, data, 0)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.send(b)
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
