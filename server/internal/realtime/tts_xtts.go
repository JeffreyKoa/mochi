package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mochi-ai/server/pkg/modelmeta"
	"github.com/mochi-ai/server/pkg/sidecarlog"
)

// xttsSynth 调用本地 Matcha X-TTS sidecar（HTTP POST /synthesize）。
type xttsSynth struct {
	baseURL string
	speed   float64
	client  *http.Client
}

// NewXttsSynth 创建本地 Matcha X-TTS 合成器（供 voice HTTP API 等复用）。
func NewXttsSynth(baseURL string, speed float64, timeout time.Duration) TTSSynthesizer {
	return newXttsSynth(baseURL, speed, timeout)
}

func newXttsSynth(baseURL string, speed float64, timeout time.Duration) TTSSynthesizer {
	if speed <= 0 {
		speed = 1.0
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &xttsSynth{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		speed:   speed,
		client:  &http.Client{Timeout: timeout},
	}
}

type xttsSession struct {
	parent *xttsSynth
	buf    strings.Builder
	mu     sync.Mutex
}

func (x *xttsSynth) StartSession(ctx context.Context, opts SynthOptions, onAudio func([]byte)) (TTSSession, error) {
	_ = ctx
	_ = opts
	_ = onAudio
	return &xttsSession{parent: x}, nil
}

func (x *xttsSynth) Synthesize(ctx context.Context, text string, opts SynthOptions, onAudio func([]byte)) error {
	modelmeta.LogCall("tts", modelmeta.VendorLocalXTTS, "matcha-zh-en", "endpoint="+x.baseURL)
	audio, err := x.request(ctx, text, opts)
	if err != nil {
		return err
	}
	if onAudio != nil && len(audio) > 0 {
		onAudio(audio)
	}
	return nil
}

func (s *xttsSession) SendText(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.WriteString(text)
	return nil
}

func (s *xttsSession) Finish(ctx context.Context) error {
	s.mu.Lock()
	text := strings.TrimSpace(s.buf.String())
	s.mu.Unlock()
	if text == "" {
		return nil
	}
	_, err := s.parent.request(ctx, text, DefaultSynthOptions())
	return err
}

func (s *xttsSession) Close() {}

func (x *xttsSynth) request(ctx context.Context, text string, opts SynthOptions) ([]byte, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}
	speed := x.speed
	if opts.Rate > 0 {
		speed = opts.Rate
	}

	body, err := json.Marshal(map[string]any{
		"text":  trimmed,
		"speed": speed,
		"pcm":   false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.baseURL+"/synthesize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := x.client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		sidecarlog.LogHTTP("xtts", http.MethodPost, x.baseURL+"/synthesize", body, 0, nil, err, elapsed)
		return nil, fmt.Errorf("xtts http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		sidecarlog.LogHTTP("xtts", http.MethodPost, x.baseURL+"/synthesize", body, resp.StatusCode, raw, err, elapsed)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		sidecarlog.LogHTTP("xtts", http.MethodPost, x.baseURL+"/synthesize", body, resp.StatusCode, raw, fmt.Errorf("xtts http %d", resp.StatusCode), elapsed)
		return nil, fmt.Errorf("xtts http %d: %s", resp.StatusCode, string(raw))
	}
	if len(raw) == 0 {
		sidecarlog.LogHTTP("xtts", http.MethodPost, x.baseURL+"/synthesize", body, resp.StatusCode, raw, fmt.Errorf("xtts empty audio"), elapsed)
		return nil, fmt.Errorf("xtts empty audio")
	}
	sidecarlog.LogHTTP("xtts", http.MethodPost, x.baseURL+"/synthesize", body, resp.StatusCode, raw, nil, elapsed)
	return raw, nil
}
