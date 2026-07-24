package edgetts

import (
	"context"
	"fmt"
	"strings"

	"github.com/difyz9/edge-tts-go/pkg/communicate"
)

const DefaultVoice = "zh-CN-XiaoyiNeural"

type Config struct {
	Voice          string
	Rate           string
	Volume         string
	Pitch          string
	Proxy          string
	ConnectTimeout int
	ReceiveTimeout int
}

func (c Config) withDefaults() Config {
	out := c
	if out.Voice == "" {
		out.Voice = DefaultVoice
	}
	if out.Rate == "" {
		out.Rate = "+0%"
	}
	if out.Volume == "" {
		out.Volume = "+0%"
	}
	if out.Pitch == "" {
		out.Pitch = "+0Hz"
	}
	if out.ConnectTimeout <= 0 {
		out.ConnectTimeout = 10
	}
	if out.ReceiveTimeout <= 0 {
		out.ReceiveTimeout = 60
	}
	return out
}

// Client synthesizes speech via Microsoft Edge online TTS (unofficial).
type Client struct {
	cfg Config
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg.withDefaults()}
}

func (c *Client) AudioFormat() string {
	return "mp3"
}

// Synthesize streams MP3 chunks to onAudio.
func (c *Client) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	comm, err := communicate.NewCommunicate(
		text,
		c.cfg.Voice,
		c.cfg.Rate,
		c.cfg.Volume,
		c.cfg.Pitch,
		c.cfg.Proxy,
		c.cfg.ConnectTimeout,
		c.cfg.ReceiveTimeout,
	)
	if err != nil {
		return fmt.Errorf("edge tts: %w", err)
	}

	chunkChan, errChan := comm.Stream(ctx)
	var chunks int
	for chunk := range chunkChan {
		if chunk.Type != "audio" || len(chunk.Data) == 0 {
			continue
		}
		chunks++
		if onAudio != nil {
			onAudio(append([]byte(nil), chunk.Data...))
		}
	}
	if err := <-errChan; err != nil {
		return fmt.Errorf("edge tts: %w", err)
	}
	if chunks == 0 {
		return fmt.Errorf("edge tts: no audio received")
	}
	return nil
}
