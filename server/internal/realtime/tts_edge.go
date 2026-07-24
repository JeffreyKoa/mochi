package realtime

import (
	"context"
	"fmt"

	"github.com/mochi-ai/server/pkg/edgetts"
)

type edgeTTSSynth struct {
	client *edgetts.Client
}

func newEdgeTTSSynth(client *edgetts.Client) TTSSynthesizer {
	return &edgeTTSSynth{client: client}
}

func (e *edgeTTSSynth) StartSession(ctx context.Context, onAudio func([]byte)) (TTSSession, error) {
	return nil, fmt.Errorf("edge tts: streaming session not supported")
}

func (e *edgeTTSSynth) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	return e.client.Synthesize(ctx, text, onAudio)
}
