package realtime

import (
	"context"

	"github.com/mochi-ai/server/pkg/dashscope"
)

type dashscopeTTSSynth struct {
	client *dashscope.TTSClient
}

func newDashscopeTTSSynth(client *dashscope.TTSClient) TTSSynthesizer {
	return &dashscopeTTSSynth{client: client}
}

type dashscopeTTSSession struct {
	sess *dashscope.TTSSession
}

func (d *dashscopeTTSSynth) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	return d.client.Synthesize(ctx, text, onAudio)
}

func (d *dashscopeTTSSynth) StartSession(ctx context.Context, onAudio func([]byte)) (TTSSession, error) {
	sess, err := d.client.StartSession(ctx, onAudio)
	if err != nil {
		return nil, err
	}
	return &dashscopeTTSSession{sess: sess}, nil
}

func (s *dashscopeTTSSession) SendText(text string) error {
	return s.sess.SendText(text)
}

func (s *dashscopeTTSSession) Finish(ctx context.Context) error {
	return s.sess.Finish(ctx)
}

func (s *dashscopeTTSSession) Close() {
	s.sess.Close()
}
