package realtime

import "context"

// ASRSession streams PCM chunks and returns the final transcript on Finish.
type ASRSession interface {
	SendAudio(pcm []byte) error
	Finish(ctx context.Context) (string, error)
	Close()
}

// ASRPartialHandler receives partial transcripts; sentenceEnd marks utterance boundaries.
type ASRPartialHandler func(text string, sentenceEnd bool)

// ASRRecognizer transcribes PCM audio to text.
type ASRRecognizer interface {
	Recognize(ctx context.Context, pcm []byte, onPartial ASRPartialHandler) (string, error)
	StartSession(ctx context.Context, onPartial ASRPartialHandler) (ASRSession, error)
}
