package tencent

import (
	"context"
	"encoding/base64"
	"fmt"
)

type ASR struct {
	client *Client
}

func NewASR(c *Client) *ASR {
	return &ASR{client: c}
}

func (a *ASR) SentenceRecognition(ctx context.Context, audio []byte, format string) (string, error) {
	if format == "" {
		format = "wav"
	}

	payload := map[string]interface{}{
		"EngSerViceType": "16k_zh",
		"SourceType":     1,
		"VoiceFormat":    format,
		"SubServiceType": 2,
		"Data":           base64.StdEncoding.EncodeToString(audio),
		"DataLen":        len(audio),
	}

	var resp struct {
		Result *string `json:"Result"`
	}
	if err := a.client.do(ctx, "asr", "SentenceRecognition", "2019-06-14", payload, &resp); err != nil {
		return "", err
	}
	if resp.Result == nil || *resp.Result == "" {
		return "", fmt.Errorf("empty asr result")
	}
	return *resp.Result, nil
}
