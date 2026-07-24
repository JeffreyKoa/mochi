package realtime

import (
	"context"
	"fmt"
	"log"
)

type fallbackSynth struct {
	primary  TTSSynthesizer
	fallback TTSSynthesizer
	primaryName string
	fallbackName string
}

func newFallbackSynth(primary, fallback TTSSynthesizer, primaryName, fallbackName string) TTSSynthesizer {
	return &fallbackSynth{
		primary:      primary,
		fallback:     fallback,
		primaryName:  primaryName,
		fallbackName: fallbackName,
	}
}

func (f *fallbackSynth) StartSession(ctx context.Context, onAudio func([]byte)) (TTSSession, error) {
	if f.primary != nil {
		return f.primary.StartSession(ctx, onAudio)
	}
	return f.fallback.StartSession(ctx, onAudio)
}

func (f *fallbackSynth) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	if f.primary != nil {
		err := f.primary.Synthesize(ctx, text, onAudio)
		if err == nil {
			return nil
		}
		log.Printf("[realtime] tts primary (%s) failed: %v; trying fallback (%s)", f.primaryName, err, f.fallbackName)
		if f.fallback == nil {
			return err
		}
	}
	if f.fallback == nil {
		return fmt.Errorf("tts not configured")
	}
	return f.fallback.Synthesize(ctx, text, onAudio)
}
