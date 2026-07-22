package realtime

import (
	"context"
	"fmt"
	"log"

	"github.com/mochi-ai/server/pkg/tencent"
)

type tencentTTSSynth struct {
	tts *tencent.TTS
}

func newTencentTTSSynth(tts *tencent.TTS) TTSSynthesizer {
	return &tencentTTSSynth{tts: tts}
}

func (t *tencentTTSSynth) StartSession(_ context.Context, _ func([]byte)) (TTSSession, error) {
	return nil, fmt.Errorf("tencent tts does not support streaming")
}

func (t *tencentTTSSynth) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	audio, _, err := t.tts.TextToVoice(ctx, text)
	if err != nil {
		return err
	}
	if len(audio) > 0 && onAudio != nil {
		onAudio(audio)
	}
	return nil
}

type fallbackTTSSynth struct {
	primary TTSSynthesizer
	backup  TTSSynthesizer
	name    string
}

func (f *fallbackTTSSynth) StartSession(ctx context.Context, onAudio func([]byte)) (TTSSession, error) {
	sess, err := f.primary.StartSession(ctx, onAudio)
	if err == nil {
		return sess, nil
	}
	if f.backup == nil {
		return nil, err
	}
	log.Printf("[realtime] %s streaming tts unavailable, will batch-synthesize after reply", f.name)
	return nil, err
}

func (f *fallbackTTSSynth) Synthesize(ctx context.Context, text string, onAudio func([]byte)) error {
	var chunks int
	wrap := func(audio []byte) {
		if len(audio) == 0 {
			return
		}
		chunks++
		if onAudio != nil {
			onAudio(audio)
		}
	}

	err := f.primary.Synthesize(ctx, text, wrap)
	if err == nil && chunks > 0 {
		return nil
	}
	if f.backup == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("primary tts returned no audio")
	}
	if err != nil {
		log.Printf("[realtime] %s primary tts failed, trying tencent fallback: %v", f.name, err)
	} else {
		log.Printf("[realtime] %s primary tts empty, trying tencent fallback", f.name)
	}
	chunks = 0
	if err := f.backup.Synthesize(ctx, text, wrap); err != nil {
		return err
	}
	if chunks == 0 {
		return fmt.Errorf("fallback tts returned no audio")
	}
	return nil
}
