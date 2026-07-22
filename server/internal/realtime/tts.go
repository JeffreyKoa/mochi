package realtime

import "context"

// TTSSynthesizer streams synthesized speech audio.
type TTSSynthesizer interface {
	StartSession(ctx context.Context, onAudio func(pcm []byte)) (TTSSession, error)
	Synthesize(ctx context.Context, text string, onAudio func(pcm []byte)) error
}

// TTSSession sends incremental text and finishes synthesis.
type TTSSession interface {
	SendText(text string) error
	Finish(ctx context.Context) error
	Close()
}
