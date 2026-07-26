//go:build !cgo

package opus

import "fmt"

// Encoder converts PCM s16le into 20ms Opus frames when built with CGO + libopus.
type Encoder struct{}

func NewEncoder(_, _ int) (*Encoder, error) {
	return nil, fmt.Errorf("opus encoder unavailable: build with CGO_ENABLED=1 and install libopus")
}

func (e *Encoder) EncodeChunk(_ []byte) ([][]byte, error) {
	return nil, fmt.Errorf("opus encoder unavailable")
}

func (e *Encoder) Flush() ([][]byte, error) {
	return nil, fmt.Errorf("opus encoder unavailable")
}
