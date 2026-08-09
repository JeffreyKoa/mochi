package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		// 批量回退按 chunk 发送，与流式路径一致，避免单次超大 PCM 包。
		if xs, ok := sess.(*xasrSession); ok {
			if err := xs.SendAudioBatch(pcm); err != nil {
				return "", err
			}
		} else if err := sess.SendAudio(pcm); err != nil {
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
		conn:        conn,
		wsURL:       x.wsURL,
		sampleRate:  x.sampleRate,
		onPartial:   onPartial,
		done:        make(chan struct{}),
		errCh:       make(chan error, 1),
		connectedAt: time.Now(),
	}
	s.resetHandshake()

	go s.readLoop()

	if err := s.sendStart(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

type xasrSession struct {
	conn        *websocket.Conn
	wsURL       string
	sampleRate  int
	onPartial   ASRPartialHandler
	done        chan struct{}
	errCh       chan error
	mu          sync.Mutex
	finalText   string
	startCh     chan struct{}
	finalCh     chan struct{}
	closeOnce   sync.Once
	connectedAt time.Time
	stats       sidecarlog.WSSessionStats
}

type xasrMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *xasrSession) resetHandshake() {
	s.startCh = make(chan struct{})
	s.finalCh = make(chan struct{})
}

func (s *xasrSession) signalStarted() {
	s.mu.Lock()
	ch := s.startCh
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (s *xasrSession) signalFinal() {
	s.mu.Lock()
	ch := s.finalCh
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (s *xasrSession) waitStarted(ctx context.Context) error {
	s.mu.Lock()
	ch := s.startCh
	s.mu.Unlock()
	select {
	case <-ch:
		return nil
	case err := <-s.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(45 * time.Second):
		return fmt.Errorf("xasr start timeout")
	}
}

func (s *xasrSession) sendStart(ctx context.Context) error {
	if err := s.sendJSON(map[string]any{
		"type":        "start",
		"sample_rate": s.sampleRate,
	}); err != nil {
		return fmt.Errorf("xasr start: %w", err)
	}
	if err := s.waitStarted(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.stats.StartedWaitMS = time.Since(s.connectedAt).Milliseconds()
	s.mu.Unlock()
	return nil
}

// Restart 在同一 WS 上开启下一轮识别，避免每句 4–5s 冷启动。
func (s *xasrSession) Restart(ctx context.Context) error {
	s.mu.Lock()
	s.finalText = ""
	s.stats.EmptyPartials = 0
	s.stats.TextPartials = 0
	s.stats.LastPartial = ""
	s.mu.Unlock()
	s.resetHandshake()
	return s.sendStart(ctx)
}

// RecognizeBatch 在同一 WS 上批量识别（handler 空流式结果回退，避免 Recognize 新建连接）。
func (s *xasrSession) RecognizeBatch(ctx context.Context, pcm []byte) (string, error) {
	if err := s.Restart(ctx); err != nil {
		return "", err
	}
	if len(pcm) > 0 {
		if err := s.SendAudioBatch(pcm); err != nil {
			return "", err
		}
	}
	return s.Finish(ctx)
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
		switch msg.Type {
		case "started":
			sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
			s.signalStarted()
		case "partial":
			text := strings.TrimSpace(msg.Text)
			s.mu.Lock()
			if text == "" {
				s.stats.EmptyPartials++
			} else {
				s.stats.TextPartials++
				s.stats.LastPartial = text
			}
			s.mu.Unlock()
			if text == "" {
				continue
			}
			sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
			if s.onPartial != nil {
				s.onPartial(text, false)
			}
		case "final":
			s.mu.Lock()
			s.finalText = msg.Text
			s.stats.FinalText = msg.Text
			s.mu.Unlock()
			sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
			s.signalFinal()
		case "error":
			sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
			select {
			case s.errCh <- fmt.Errorf("xasr: %s", msg.Text):
			default:
			}
			return
		default:
			sidecarlog.LogWSInbound("xasr", s.wsURL, msg.Type, msg)
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

// SendAudio 流式发送单个 PCM chunk。
func (s *xasrSession) SendAudio(pcm []byte) error {
	return s.sendPCM(pcm, "stream")
}

// SendAudioBatch 批量识别：按 sidecar 推荐 chunk 分片发送。
func (s *xasrSession) SendAudioBatch(pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	chunk := sidecarlog.WSBatchChunkBytes()
	sidecarlog.LogWSBatchPCMStart("xasr", s.wsURL, len(pcm), chunk)
	for off := 0; off < len(pcm); off += chunk {
		end := off + chunk
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := s.sendPCM(pcm[off:end], "batch"); err != nil {
			return err
		}
	}
	return nil
}

func (s *xasrSession) sendPCM(pcm []byte, mode string) error {
	if len(pcm) == 0 {
		return nil
	}
	s.mu.Lock()
	s.stats.AudioChunks++
	s.stats.AudioBytes += len(pcm)
	s.mu.Unlock()
	sidecarlog.LogWSOutboundPCM("xasr", s.wsURL, len(pcm), mode)
	return s.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

func (s *xasrSession) Finish(ctx context.Context) (string, error) {
	s.mu.Lock()
	s.finalText = ""
	s.finalCh = make(chan struct{})
	finalCh := s.finalCh
	s.mu.Unlock()

	if err := s.sendJSON(map[string]any{"type": "end"}); err != nil {
		return "", fmt.Errorf("xasr end: %w", err)
	}

	select {
	case <-finalCh:
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
		s.mu.Lock()
		st := s.stats
		st.ElapsedMS = time.Since(s.connectedAt).Milliseconds()
		s.mu.Unlock()
		sidecarlog.LogWSSessionSummary("xasr", s.wsURL, "close", st)
		_ = s.conn.Close()
	})
}
