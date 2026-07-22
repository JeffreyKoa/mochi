package realtime

import "context"

// ASRSession streams PCM chunks and returns the final transcript on Finish.
type ASRSession interface {
	SendAudio(pcm []byte) error
	Finish(ctx context.Context) (string, error)
	Close()
}

// ASRRecognizer transcribes PCM audio to text.
type ASRRecognizer interface {
	Recognize(ctx context.Context, pcm []byte, onPartial func(text string)) (string, error)
	StartSession(ctx context.Context, onPartial func(text string)) (ASRSession, error)
}
