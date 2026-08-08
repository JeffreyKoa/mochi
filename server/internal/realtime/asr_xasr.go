package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mochi-ai/server/pkg/modelmeta"
	"github.com/mochi-ai/server/pkg/sidecarlog"
)

// xasrASR 对接本机 sherpa_streaming_server（与 Desktop XAsrClient 同协议）。
type xasrASR struct {
	wsURL      string
	sampleRate int
	model      string
}

func newXasrASR(wsURL string, sampleRate int, model string) ASRRecognizer {
	if sampleRate == 0 {
		sampleRate = 16000
	}
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:8766"
	}
	if model == "" {
		model = "sherpa-streaming-zh"
	}
	return &xasrASR{wsURL: wsURL, sampleRate: sampleRate, model: model}
}

// NewXasrASR 创建 x-asr sidecar 识别器（供 xasrprobe / 测试）。
func NewXasrASR(wsURL string, sampleRate int) ASRRecognizer {
	return newXasrASR(wsURL, sampleRate, "sherpa-streaming-zh")
}

func (x *xasrASR) Recognize(ctx context.Context, pcm []byte, onPartial ASRPartialHandler) (string, error) {
	sess, err := x.StartSession(ctx, onPartial)
	if err != nil {
		return "", err
	}
	defer sess.Close()
	if len(pcm) > 0 {
		if err := sess.SendAudio(pcm); err != nil {
			return "", err
		}
	}
	return sess.Finish(ctx)
}

func (x *xasrASR) StartSession(ctx context.Context, onPartial ASRPartialHandler) (ASRSession, error) {
	modelmeta.LogCall("asr", modelmeta.VendorLocalXASR, x.model, "endpoint="+x.wsURL)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, x.wsURL, http.Header{})
	sidecarlog.LogWSConnect("xasr", x.wsURL, err)
	if err != nil {
		return nil, fmt.Errorf("xasr dial: %w", err)
	}

	s := &xasrSession{
		conn:       conn,
		wsURL:      x.wsURL,
		onPartial:  onPartial,
		started:    make(chan struct{}),
		done:       make(chan struct{}),
		errCh:      make(chan error, 1),
	}

	go s.readLoop()

	if err := s.sendJSON(map[string]any{
		"type":        "start",
		"sample_rate": x.sampleRate,
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("xasr start: %w", err)
	}

	select {
	case <-s.started:
		return s, nil
	case err := <-s.errCh:
		conn.Close()
		return nil, err
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	case <-time.After(45 * time.Second):
		conn.Close()
		return nil, fmt.Errorf("xasr start timeout")
	}
}

type xasrSession struct {
	conn      *websocket.Conn
	wsURL     string
	onPartial ASRPartialHandler
	started   chan struct{}
	done      chan struct{}
	errCh     chan error
	mu        sync.Mutex
	finalText string
	startOnce sync.Once
	closeOnce sync.Once
}

type xasrMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *xasrSession) readLoop() {
	defer close(s.done)
	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case s.errCh <- err:
			default:
			}
			return
		}
		var msg xasrMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			sidecarlog.LogWSInbound("xasr", s.wsURL, "raw", string(data))
			select {
			case s.errCh <- fmt.Errorf("xasr json: %w raw=%q", err, string(data)):
			default:
			}
			continue
		}
		sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
		switch msg.Type {
		case "started":
			s.startOnce.Do(func() { close(s.started) })
		case "partial":
			if msg.Text != "" && s.onPartial != nil {
				s.onPartial(msg.Text, false)
			}
		case "final":
			s.mu.Lock()
			s.finalText = msg.Text
			s.mu.Unlock()
			return
		case "error":
			select {
			case s.errCh <- fmt.Errorf("xasr: %s", msg.Text):
			default:
			}
			return
		}
	}
}

func (s *xasrSession) sendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sidecarlog.LogWSOutbound("xasr", s.wsURL, "json", v)
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

func (s *xasrSession) SendAudio(pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	sidecarlog.LogWSOutbound("xasr", s.wsURL, "audio", fmt.Sprintf("<pcm bytes=%d>", len(pcm)))
	return s.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

func (s *xasrSession) Finish(ctx context.Context) (string, error) {
	if err := s.sendJSON(map[string]any{"type": "end"}); err != nil {
		return "", fmt.Errorf("xasr end: %w", err)
	}

	select {
	case <-s.done:
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-s.errCh:
		s.mu.Lock()
		text := s.finalText
		s.mu.Unlock()
		if text != "" {
			return text, nil
		}
		return "", err
	case <-time.After(30 * time.Second):
		s.mu.Lock()
		text := s.finalText
		s.mu.Unlock()
		if text != "" {
			return text, nil
		}
		return "", fmt.Errorf("xasr final timeout")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finalText, nil
}

func (s *xasrSession) Close() {
	s.closeOnce.Do(func() {
		_ = s.conn.Close()
	})
}
