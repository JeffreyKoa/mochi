package realtime

import (
	"context"

	"github.com/mochi-ai/server/pkg/dashscope"
)

// TTSSynthesizer streams synthesized speech audio.
type TTSSynthesizer interface {
	StartSession(ctx context.Context, opts dashscope.SynthOptions, onAudio func(pcm []byte)) (TTSSession, error)
	Synthesize(ctx context.Context, text string, opts dashscope.SynthOptions, onAudio func(pcm []byte)) error
}

// TTSSession sends incremental text and finishes synthesis.
type TTSSession interface {
	SendText(text string) error
	Finish(ctx context.Context) error
	Close()
}
