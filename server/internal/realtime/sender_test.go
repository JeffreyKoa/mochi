package realtime

import (
	"encoding/binary"
	"testing"
)

func TestConnSender_SendTTSAudioBinary(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		formatByte byte
		seq        int64
	}{
		{name: "mp3", format: "mp3", formatByte: 0x01, seq: 42},
		{name: "pcm", format: "pcm", formatByte: 0x02, seq: 99},
	}

	rawAudio := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentMsg WSMessage
			sender := &connSender{
				send: func(msg WSMessage) error {
					sentMsg = msg
					return nil
				},
			}

			err := sender.SendTTSAudioBinary(rawAudio, tt.format, tt.seq)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !sentMsg.IsBinary {
				t.Errorf("expected IsBinary true")
			}

			if len(sentMsg.Data) != 10+len(rawAudio) {
				t.Fatalf("expected payload length %d, got %d", 10+len(rawAudio), len(sentMsg.Data))
			}

			if sentMsg.Data[0] != 0x01 {
				t.Errorf("expected MsgType 0x01, got 0x%02x", sentMsg.Data[0])
			}

			if sentMsg.Data[1] != tt.formatByte {
				t.Errorf("expected Format 0x%02x, got 0x%02x", tt.formatByte, sentMsg.Data[1])
			}

			seq := binary.BigEndian.Uint64(sentMsg.Data[2:10])
			if seq != uint64(tt.seq) {
				t.Errorf("expected Seq %d, got %d", tt.seq, seq)
			}

			payload := sentMsg.Data[10:]
			if string(payload) != string(rawAudio) {
				t.Errorf("audio payload mismatch")
			}
		})
	}
}
