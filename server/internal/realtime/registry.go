package realtime

import (
	"sync"
)

type sessionSender struct {
	send    func(WSMessage) error
	onEvict func()
}

type Registry struct {
	mu             sync.RWMutex
	sessions       map[uint64]map[string]sessionSender
	userProcessing map[uint64]int // voice 回合 in-flight 计数（life_nudge 等需避让）
}

func NewRegistry() *Registry {
	return &Registry{
		sessions:       make(map[uint64]map[string]sessionSender),
		userProcessing: make(map[uint64]int),
	}
}

// SetUserProcessing 标记用户是否有 voice 管线正在处理（ASR/LLM/TTS）。
func (r *Registry) SetUserProcessing(userID uint64, active bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if active {
		r.userProcessing[userID]++
		return
	}
	if r.userProcessing[userID] <= 1 {
		delete(r.userProcessing, userID)
		return
	}
	r.userProcessing[userID]--
}

// IsUserProcessing 用户是否处于 voice 处理中。
func (r *Registry) IsUserProcessing(userID uint64) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.userProcessing[userID] > 0
}

func (r *Registry) Register(userID uint64, sessionID string, send func(WSMessage) error, onEvict func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[userID] == nil {
		r.sessions[userID] = make(map[string]sessionSender)
	} else {
		for id, s := range r.sessions[userID] {
			if id != sessionID {
				if s.onEvict != nil {
					s.onEvict()
				}
				delete(r.sessions[userID], id)
			}
		}
	}
	r.sessions[userID][sessionID] = sessionSender{send: send, onEvict: onEvict}
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
		if err := s.send(WSMessage{IsBinary: false, Data: raw}); err == nil {
			delivered++
		}
	}
	return delivered
}
