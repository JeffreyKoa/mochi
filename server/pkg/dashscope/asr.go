package dashscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ASRClient calls Paraformer realtime over WebSocket.
type ASRClient struct {
	apiKey      string
	model       string
	sampleRate  int
	wsURL       string
	workspaceID string
}

// EndpointConfig configures DashScope WebSocket routing (cn-beijing workspace URL, etc.).
type EndpointConfig struct {
	WSURL       string
	WorkspaceID string
	Region      string
}

func NewASRClient(apiKey, model string, sampleRate int, ep ...EndpointConfig) *ASRClient {
	if model == "" {
		model = "paraformer-realtime-v2"
	}
	if sampleRate == 0 {
		sampleRate = 16000
	}
	var cfg EndpointConfig
	if len(ep) > 0 {
		cfg = ep[0]
	}
	return &ASRClient{
		apiKey:      apiKey,
		model:       model,
		sampleRate:  sampleRate,
		wsURL:       ResolveWSURL(cfg.WSURL, cfg.WorkspaceID, cfg.Region),
		workspaceID: cfg.WorkspaceID,
	}
}

// ASRPartialHandler is called for each partial transcript; sentenceEnd is true at utterance boundaries.
type ASRPartialHandler func(text string, sentenceEnd bool)

// ASRSession holds an active recognition task for streaming input.
type ASRSession struct {
	conn       *websocket.Conn
	taskID     string
	onPartial  ASRPartialHandler
	started    chan struct{}
	finished   chan struct{}
	errCh      chan error
	finishSent chan struct{}
	mu         sync.Mutex
	finalText  string
	once       sync.Once
}

// StartSession opens an ASR task for incremental audio input.
func (c *ASRClient) StartSession(ctx context.Context, onPartial ASRPartialHandler) (*ASRSession, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("dashscope api key not configured")
	}

	taskID := uuid.NewString()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.apiKey)
	if c.workspaceID != "" {
		header.Set("X-DashScope-WorkSpace", c.workspaceID)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, c.wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("asr dial: %w", err)
	}

	s := &ASRSession{
		conn:       conn,
		taskID:     taskID,
		onPartial:  onPartial,
		started:    make(chan struct{}),
		finished:   make(chan struct{}),
		errCh:      make(chan error, 1),
		finishSent: make(chan struct{}),
	}

	go c.readLoop(conn, onPartial, s.started, s.finished, s.errCh, s.finishSent, &s.mu, &s.finalText)

	runTask := map[string]any{
		"header": map[string]any{
			"action":    "run-task",
			"task_id":   taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"task_group": "audio",
			"task":       "asr",
			"function":   "recognition",
			"model":      c.model,
			"parameters": map[string]any{
				"format":         "pcm",
				"sample_rate":    c.sampleRate,
				"language_hints": []string{"zh", "en"},
			},
			"input": map[string]any{},
		},
	}
	if err := writeJSON(conn, runTask); err != nil {
		conn.Close()
		return nil, err
	}

	select {
	case <-s.started:
		return s, nil
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	case <-time.After(20 * time.Second):
		conn.Close()
		return nil, fmt.Errorf("asr task start timeout")
	case err := <-s.errCh:
		conn.Close()
		return nil, err
	}
}

func (s *ASRSession) SendAudio(pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

func (s *ASRSession) Finish(ctx context.Context) (string, error) {
	const chunkSize = 3200
	silence := make([]byte, chunkSize*3)
	if err := s.conn.WriteMessage(websocket.BinaryMessage, silence); err != nil {
		return "", fmt.Errorf("asr send silence: %w", err)
	}

	close(s.finishSent)

	finishTask := map[string]any{
		"header": map[string]any{
			"action":    "finish-task",
			"task_id":   s.taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"input": map[string]any{},
		},
	}
	if err := writeJSON(s.conn, finishTask); err != nil {
		return "", err
	}

	select {
	case <-s.finished:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(45 * time.Second):
		s.mu.Lock()
		text := s.finalText
		s.mu.Unlock()
		if text != "" {
			return text, nil
		}
		return "", fmt.Errorf("asr result timeout")
	case err := <-s.errCh:
		s.mu.Lock()
		text := s.finalText
		s.mu.Unlock()
		if text != "" {
			return text, nil
		}
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finalText, nil
}

func (s *ASRSession) Close() {
	s.once.Do(func() {
		_ = s.conn.Close()
	})
}

// Recognize streams PCM int16 LE mono audio and returns the final transcript.
func (c *ASRClient) Recognize(ctx context.Context, pcm []byte, onPartial ASRPartialHandler) (string, error) {
	if len(pcm) == 0 {
		return "", fmt.Errorf("empty audio")
	}

	sess, err := c.StartSession(ctx, onPartial)
	if err != nil {
		return "", err
	}
	defer sess.Close()

	const chunkSize = 3200 // 100ms @ 16kHz mono int16
	chunkDelay := 50 * time.Millisecond
	for i := 0; i < len(pcm); i += chunkSize {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		end := i + chunkSize
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := sess.SendAudio(pcm[i:end]); err != nil {
			return "", fmt.Errorf("asr send audio: %w", err)
		}
		if chunkDelay > 0 && end < len(pcm) {
			time.Sleep(chunkDelay)
		}
	}

	return sess.Finish(ctx)
}

func (c *ASRClient) sendAudioPaced(ctx context.Context, conn *websocket.Conn, pcm []byte, chunkSize int, delay time.Duration) error {
	for i := 0; i < len(pcm); i += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		end := i + chunkSize
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, pcm[i:end]); err != nil {
			return fmt.Errorf("asr send audio: %w", err)
		}
		if delay > 0 && end < len(pcm) {
			time.Sleep(delay)
		}
	}
	return nil
}

func (c *ASRClient) readLoop(
	conn *websocket.Conn,
	onPartial ASRPartialHandler,
	started chan struct{},
	finished chan struct{},
	errCh chan error,
	finishSent <-chan struct{},
	mu *sync.Mutex,
	finalText *string,
) {
	var startOnce sync.Once
	var finishOnce sync.Once
	signalDone := func() {
		finishOnce.Do(func() { close(finished) })
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-finishSent:
				mu.Lock()
				hasText := *finalText != ""
				mu.Unlock()
				if hasText {
					signalDone()
					return
				}
			default:
			}
			select {
			case errCh <- err:
			default:
			}
			return
		}

		var msg struct {
			Header struct {
				Event        string `json:"event"`
				ErrorMessage string `json:"error_message"`
			} `json:"header"`
			Payload struct {
				Output struct {
					Sentence struct {
						Text        string `json:"text"`
						SentenceEnd bool   `json:"sentence_end"`
						Heartbeat   bool   `json:"heartbeat"`
					} `json:"sentence"`
				} `json:"output"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Header.Event {
		case "task-started":
			startOnce.Do(func() { close(started) })
		case "result-generated":
			if msg.Payload.Output.Sentence.Heartbeat {
				continue
			}
			text := msg.Payload.Output.Sentence.Text
			if text == "" {
				continue
			}
			mu.Lock()
			*finalText = text
			mu.Unlock()
			if onPartial != nil {
				onPartial(text, msg.Payload.Output.Sentence.SentenceEnd)
			}
			if msg.Payload.Output.Sentence.SentenceEnd {
				select {
				case <-finishSent:
					signalDone()
					return
				default:
				}
			}
		case "task-finished":
			signalDone()
			return
		case "task-failed":
			select {
			case errCh <- fmt.Errorf("asr failed: %s", msg.Header.ErrorMessage):
			default:
			}
			return
		}
	}
}

func writeJSON(conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}
