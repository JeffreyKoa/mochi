package tencent

import (
	"context"
	"encoding/base64"
	"fmt"
)

type TTS struct {
	client    *Client
	voiceType int64
}

func NewTTS(c *Client, voiceType int64) *TTS {
	if voiceType == 0 {
		voiceType = 101001
	}
	return &TTS{client: c, voiceType: voiceType}
}

func (t *TTS) TextToVoice(ctx context.Context, text string) ([]byte, string, error) {
	payload := map[string]interface{}{
		"Text":      text,
		"SessionId": fmt.Sprintf("mochi-%d", len(text)),
		"ModelType": 1,
		"VoiceType": t.voiceType,
		"Codec":     "mp3",
	}

	var resp struct {
		Audio *string `json:"Audio"`
	}
	if err := t.client.do(ctx, "tts", "TextToVoice", "2019-08-23", payload, &resp); err != nil {
		return nil, "", err
	}
	if resp.Audio == nil || *resp.Audio == "" {
		return nil, "", fmt.Errorf("empty tts audio")
	}

	audio, err := base64.StdEncoding.DecodeString(*resp.Audio)
	if err != nil {
		return nil, "", err
	}
	return audio, "mp3", nil
}
