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

// TTSClient streams Qwen-Audio-TTS / CosyVoice synthesis over WebSocket.
type TTSClient struct {
	apiKey      string
	model       string
	voice       string
	sampleRate  int
	wsURL       string
	workspaceID string
	audioFormat string
}

func NewTTSClient(apiKey, model, voice string, sampleRate int, ep ...EndpointConfig) *TTSClient {
	if model == "" {
		model = "qwen-audio-3.0-tts-plus"
	}
	if voice == "" {
		voice = "longanhuan_v3.6"
	}
	if sampleRate == 0 {
		sampleRate = 22050
	}
	var cfg EndpointConfig
	if len(ep) > 0 {
		cfg = ep[0]
	}
	return &TTSClient{
		apiKey:      apiKey,
		model:       model,
		voice:       voice,
		sampleRate:  sampleRate,
		wsURL:       ResolveWSURL(cfg.WSURL, cfg.WorkspaceID, cfg.Region),
		workspaceID: cfg.WorkspaceID,
		audioFormat: "mp3",
	}
}

// AudioFormat returns the configured output format (mp3 or pcm).
func (c *TTSClient) AudioFormat() string {
	if c.audioFormat == "" {
		return "mp3"
	}
	return c.audioFormat
}

// TTSSession holds an active synthesis task.
type TTSSession struct {
	conn     *websocket.Conn
	taskID   string
	onAudio  func([]byte)
	started  chan struct{}
	finished chan struct{}
	errCh    chan error
	once     sync.Once
}

// StartSession opens a TTS task and begins receiving audio chunks.
func (c *TTSClient) StartSession(ctx context.Context, onAudio func([]byte)) (*TTSSession, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("dashscope api key not configured")
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.apiKey)
	if c.workspaceID != "" {
		header.Set("X-DashScope-WorkSpace", c.workspaceID)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, _, err := dialer.DialContext(dialCtx, c.wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("tts dial: %w", err)
	}

	s := &TTSSession{
		conn:     conn,
		taskID:   uuid.NewString(),
		onAudio:  onAudio,
		started:  make(chan struct{}),
		finished: make(chan struct{}),
		errCh:    make(chan error, 1),
	}

	go s.readLoop()

	format := c.AudioFormat()
	params := map[string]any{
		"text_type":   "PlainText",
		"voice":       c.voice,
		"format":      format,
		"sample_rate": c.sampleRate,
		"volume":      50,
		"rate":        1.0,
		"pitch":       1.0,
		"enable_ssml": false,
	}

	runTask := map[string]any{
		"header": map[string]any{
			"action":    "run-task",
			"task_id":   s.taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"task_group": "audio",
			"task":       "tts",
			"function":   "SpeechSynthesizer",
			"model":      c.model,
			"parameters": params,
			"input":      map[string]any{},
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
	case <-time.After(15 * time.Second):
		conn.Close()
		return nil, fmt.Errorf("tts task start timeout")
	case err := <-s.errCh:
		conn.Close()
		return nil, err
	}
}

func (s *TTSSession) SendText(text string) error {
	if text == "" {
		return nil
	}
	msg := map[string]any{
		"header": map[string]any{
			"action":    "continue-task",
			"task_id":   s.taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"input": map[string]any{
				"text": text,
			},
		},
	}
	return writeJSON(s.conn, msg)
}

func (s *TTSSession) Finish(ctx context.Context) error {
	msg := map[string]any{
		"header": map[string]any{
			"action":    "finish-task",
			"task_id":   s.taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"input": map[string]any{},
		},
	}
	if err := writeJSON(s.conn, msg); err != nil {
		return err
	}

	select {
	case <-s.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(60 * time.Second):
		return fmt.Errorf("tts finish timeout")
	case err := <-s.errCh:
		return err
	}
}

func (s *TTSSession) Close() {
	s.once.Do(func() {
		_ = s.conn.Close()
	})
}

func (s *TTSSession) readLoop() {
	defer s.Close()

	for {
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case s.errCh <- err:
			default:
			}
			return
		}

		if msgType == websocket.BinaryMessage {
			if len(data) > 0 && s.onAudio != nil {
				s.onAudio(data)
			}
			continue
		}

		var msg struct {
			Header struct {
				Event        string `json:"event"`
				ErrorMessage string `json:"error_message"`
			} `json:"header"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Header.Event {
		case "task-started":
			select {
			case <-s.started:
			default:
				close(s.started)
			}
		case "task-finished":
			select {
			case <-s.finished:
			default:
				close(s.finished)
			}
			return
		case "task-failed":
			select {
			case s.errCh <- fmt.Errorf("tts failed: %s", msg.Header.ErrorMessage):
			default:
			}
			return
		}
	}
}

// Synthesize converts full text to audio in one session (non-incremental helper).
func (c *TTSClient) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	sess, err := c.StartSession(ctx, onAudio)
	if err != nil {
		return err
	}
	defer sess.Close()

	if err := sess.SendText(text); err != nil {
		return err
	}
	return sess.Finish(ctx)
}
