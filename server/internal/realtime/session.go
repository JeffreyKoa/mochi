package realtime

import (
	"context"
	"sync"
)

type SessionState string

const (
	StateIdle      SessionState = "idle"
	StateListening SessionState = "listening"
	StateThinking  SessionState = "thinking"
	StateSpeaking  SessionState = "speaking"
)

type Session struct {
	ID     string
	UserID uint64

	mu    sync.Mutex
	state SessionState

	audioSeq int64
	ttsSeq   int64

	pipelineMu     sync.Mutex
	pipelineCancel context.CancelFunc

	onStateChange func(SessionState)
}

func NewSession(id string, userID uint64, onStateChange func(SessionState)) *Session {
	return &Session{
		ID:            id,
		UserID:        userID,
		state:         StateIdle,
		onStateChange: onStateChange,
	}
}

func (s *Session) State() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) SetState(next SessionState) {
	s.mu.Lock()
	if s.state == next {
		s.mu.Unlock()
		return
	}
	s.state = next
	cb := s.onStateChange
	s.mu.Unlock()
	if cb != nil {
		cb(next)
	}
}

func (s *Session) NextAudioSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audioSeq++
	return s.audioSeq
}

func (s *Session) NextTTSSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttsSeq++
	return s.ttsSeq
}

// BeginPipeline returns a cancellable context for an active reply pipeline.
func (s *Session) BeginPipeline(parent context.Context) context.Context {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	if s.pipelineCancel != nil {
		s.pipelineCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.pipelineCancel = cancel
	return ctx
}

// CancelPipeline stops the active reply pipeline (barge-in).
func (s *Session) CancelPipeline() {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	if s.pipelineCancel != nil {
		s.pipelineCancel()
		s.pipelineCancel = nil
	}
}

// EndPipeline clears the pipeline cancel handle after normal completion.
func (s *Session) EndPipeline() {
	s.pipelineMu.Lock()
	defer s.pipelineMu.Unlock()
	s.pipelineCancel = nil
}
