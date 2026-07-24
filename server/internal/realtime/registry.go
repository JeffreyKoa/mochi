package realtime

import (
	"sync"
)

type sessionSender struct {
	send func([]byte) error
}

type Registry struct {
	mu       sync.RWMutex
	sessions map[uint64]map[string]sessionSender
}

func NewRegistry() *Registry {
	return &Registry{sessions: make(map[uint64]map[string]sessionSender)}
}

func (r *Registry) Register(userID uint64, sessionID string, send func([]byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[userID] == nil {
		r.sessions[userID] = make(map[string]sessionSender)
	}
	r.sessions[userID][sessionID] = sessionSender{send: send}
}

func (r *Registry) Unregister(userID uint64, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.sessions[userID]
	if m == nil {
		return
	}
	delete(m, sessionID)
	if len(m) == 0 {
		delete(r.sessions, userID)
	}
}

func (r *Registry) SendToUser(userID uint64, msgType string, data any) int {
	raw, err := marshalMsg(msgType, data, 0)
	if err != nil {
		return 0
	}

	r.mu.RLock()
	sessions := r.sessions[userID]
	copies := make([]sessionSender, 0, len(sessions))
	for _, s := range sessions {
		copies = append(copies, s)
	}
	r.mu.RUnlock()

	delivered := 0
	for _, s := range copies {
		if s.send == nil {
			continue
		}
		if err := s.send(raw); err == nil {
			delivered++
		}
	}
	return delivered
}
