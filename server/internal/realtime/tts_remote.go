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

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/modelmeta"
)

// remoteTTS 通过 OpenAI 兼容 POST /audio/speech 合成语音。
type remoteTTS struct {
	apiBase string
	apiKey  string
	model   string
	voice   string
	client  *http.Client
}

func newRemoteTTS(ai config.AIConfig) TTSSynthesizer {
	timeout := 60 * time.Second
	return &remoteTTS{
		apiBase: strings.TrimRight(strings.TrimSpace(ai.APIBase), "/"),
		apiKey:  strings.TrimSpace(ai.APIKey),
		model:   strings.TrimSpace(ai.TTSModel),
		voice:   strings.TrimSpace(ai.TTSVoice),
		client:  &http.Client{Timeout: timeout},
	}
}

type remoteTTSSession struct {
	parent *remoteTTS
	buf    strings.Builder
	mu     sync.Mutex
}

func (r *remoteTTS) StartSession(ctx context.Context, opts SynthOptions, onAudio func([]byte)) (TTSSession, error) {
	_ = ctx
	_ = opts
	_ = onAudio
	return &remoteTTSSession{parent: r}, nil
}

func (r *remoteTTS) Synthesize(ctx context.Context, text string, opts SynthOptions, onAudio func([]byte)) error {
	audio, err := r.request(ctx, text)
	if err != nil {
		return err
	}
	if onAudio != nil && len(audio) > 0 {
		onAudio(audio)
	}
	return nil
}

func (s *remoteTTSSession) SendText(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.WriteString(text)
	return nil
}

func (s *remoteTTSSession) Finish(ctx context.Context) error {
	s.mu.Lock()
	text := strings.TrimSpace(s.buf.String())
	s.mu.Unlock()
	if text == "" {
		return nil
	}
	audio, err := s.parent.request(ctx, text)
	if err != nil {
		return err
	}
	_ = audio
	return nil
}

func (s *remoteTTSSession) Close() {}

func (r *remoteTTS) request(ctx context.Context, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}
	if r.apiKey == "" {
		return nil, fmt.Errorf("remote TTS: ai.api_key not configured")
	}
	modelmeta.LogCall("tts", modelmeta.InferVendorFromAPIBase(r.apiBase), r.model)

	payload := map[string]string{
		"model": r.model,
		"input": text,
		"voice": r.voice,
	}
	body, _ := json.Marshal(payload)
	url := r.apiBase + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote tts: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote tts http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}
