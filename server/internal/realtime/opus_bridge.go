package realtime

import (
	"log"

	"github.com/mochi-ai/server/pkg/opus"
)

type OpusBridge struct {
	encoder    *opus.Encoder
	sampleRate int
	bitrate    int
}

func NewOpusBridge(inputSampleRate, targetBitrate int) (*OpusBridge, error) {
	enc, err := opus.NewEncoder(inputSampleRate, targetBitrate)
	if err != nil {
		return nil, err
	}
	return &OpusBridge{
		encoder:    enc,
		sampleRate: 48000,
		bitrate:    targetBitrate,
	}, nil
}

func (b *OpusBridge) EncodeChunk(pcmBytes []byte) ([][]byte, error) {
	if b == nil || b.encoder == nil {
		return nil, nil
	}
	return b.encoder.EncodeChunk(pcmBytes)
}

func (b *OpusBridge) Flush() ([][]byte, error) {
	if b == nil || b.encoder == nil {
		return nil, nil
	}
	return b.encoder.Flush()
}

func (b *OpusBridge) SendStreamStart(send Sender) {
	if send == nil {
		return
	}
	_ = send.Send(MsgTTSStreamStart, TTSStreamStart{
		Codec:      "opus",
		SampleRate: 48000,
		Channels:   1,
		FrameMS:    20,
		Bitrate:    b.bitrate,
	})
	log.Printf("[realtime] sent tts_stream_start: codec=opus rate=48000 channels=1 frame_ms=20 bitrate=%d", b.bitrate)
}
