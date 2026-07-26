//go:build cgo

package opus

import (
	"fmt"
	"sync"

	gopus "gopkg.in/hraban/opus.v2"
)

// Encoder converts PCM s16le (22050Hz/24000Hz/48000Hz) into 20ms Opus frames (48kHz Mono).
type Encoder struct {
	mu              sync.Mutex
	inputSampleRate int
	opusEncoder     *gopus.Encoder
	pcmBuffer       []int16
	frameSamples    int
	bitrate         int
}

func NewEncoder(inputSampleRate, targetBitrate int) (*Encoder, error) {
	if inputSampleRate <= 0 {
		inputSampleRate = 22050
	}
	if targetBitrate <= 0 {
		targetBitrate = 24000
	}

	enc, err := gopus.NewEncoder(48000, 1, gopus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}
	_ = enc.SetBitrate(targetBitrate)

	return &Encoder{
		inputSampleRate: inputSampleRate,
		opusEncoder:     enc,
		pcmBuffer:       make([]int16, 0, 4800),
		frameSamples:    960,
		bitrate:         targetBitrate,
	}, nil
}

func (e *Encoder) EncodeChunk(pcmBytes []byte) ([][]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(pcmBytes) == 0 {
		return nil, nil
	}

	numInputSamples := len(pcmBytes) / 2
	inputSamples := make([]int16, numInputSamples)
	for i := 0; i < numInputSamples; i++ {
		inputSamples[i] = int16(uint16(pcmBytes[i*2]) | uint16(pcmBytes[i*2+1])<<8)
	}

	resampled := ResampleLinear(inputSamples, e.inputSampleRate, 48000)
	e.pcmBuffer = append(e.pcmBuffer, resampled...)

	var packets [][]byte
	outBuf := make([]byte, 1000)

	for len(e.pcmBuffer) >= e.frameSamples {
		frame := e.pcmBuffer[:e.frameSamples]
		n, err := e.opusEncoder.Encode(frame, outBuf)
		if err != nil {
			return nil, fmt.Errorf("opus encode: %w", err)
		}
		packet := make([]byte, n)
		copy(packet, outBuf[:n])
		packets = append(packets, packet)
		e.pcmBuffer = e.pcmBuffer[e.frameSamples:]
	}

	return packets, nil
}

func (e *Encoder) Flush() ([][]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.pcmBuffer) == 0 {
		return nil, nil
	}

	var packets [][]byte
	outBuf := make([]byte, 1000)

	padNeeded := e.frameSamples - len(e.pcmBuffer)
	frame := append(e.pcmBuffer, make([]int16, padNeeded)...)

	n, err := e.opusEncoder.Encode(frame, outBuf)
	if err == nil && n > 0 {
		packet := make([]byte, n)
		copy(packet, outBuf[:n])
		packets = append(packets, packet)
	}

	e.pcmBuffer = e.pcmBuffer[:0]
	return packets, nil
}
