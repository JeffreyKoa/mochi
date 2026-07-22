package realtime

import (
	"context"

	"github.com/mochi-ai/server/pkg/dashscope"
)

type dashscopeASR struct {
	client *dashscope.ASRClient
}

func newDashscopeASR(client *dashscope.ASRClient) ASRRecognizer {
	return &dashscopeASR{client: client}
}

func (d *dashscopeASR) Recognize(ctx context.Context, pcm []byte, onPartial func(text string)) (string, error) {
	return d.client.Recognize(ctx, pcm, onPartial)
}

func (d *dashscopeASR) StartSession(ctx context.Context, onPartial func(text string)) (ASRSession, error) {
	sess, err := d.client.StartSession(ctx, onPartial)
	if err != nil {
		return nil, err
	}
	return &dashscopeASRSession{sess: sess}, nil
}

type dashscopeASRSession struct {
	sess *dashscope.ASRSession
}

func (s *dashscopeASRSession) SendAudio(pcm []byte) error {
	return s.sess.SendAudio(pcm)
}

func (s *dashscopeASRSession) Finish(ctx context.Context) (string, error) {
	return s.sess.Finish(ctx)
}

func (s *dashscopeASRSession) Close() {
	s.sess.Close()
}
