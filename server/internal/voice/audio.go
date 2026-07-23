package voice

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// audioToPCM converts uploaded audio to mono PCM int16 LE for Dashscope ASR.
func audioToPCM(data []byte, format string) ([]byte, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "pcm", "raw":
		return data, nil
	case "wav":
		return wavPCM(data)
	default:
		return nil, fmt.Errorf("unsupported audio format %q (use wav or pcm)", format)
	}
}

func wavPCM(data []byte) ([]byte, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("wav too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("invalid wav header")
	}

	var audioFormat, numChannels, bitsPerSample uint16
	var sampleRate uint32
	var pcm []byte

	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if offset+chunkSize > len(data) {
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkSize >= 16 {
				audioFormat = binary.LittleEndian.Uint16(data[offset:])
				numChannels = binary.LittleEndian.Uint16(data[offset+2:])
				sampleRate = binary.LittleEndian.Uint32(data[offset+4:])
				bitsPerSample = binary.LittleEndian.Uint16(data[offset+14:])
			}
		case "data":
			pcm = append([]byte(nil), data[offset:offset+chunkSize]...)
		}
		offset += chunkSize + chunkSize%2
	}

	if len(pcm) == 0 {
		return nil, fmt.Errorf("wav contains no pcm data")
	}
	if audioFormat != 1 {
		return nil, fmt.Errorf("wav must be pcm format (got format=%d)", audioFormat)
	}
	if bitsPerSample != 16 {
		return nil, fmt.Errorf("wav must be 16-bit (got %d)", bitsPerSample)
	}
	if sampleRate != 16000 {
		return nil, fmt.Errorf("wav must be 16kHz for ASR (got %dHz)", sampleRate)
	}
	if numChannels == 1 {
		return pcm, nil
	}
	if numChannels == 2 {
		return stereoToMono(pcm), nil
	}
	return nil, fmt.Errorf("wav channels=%d not supported", numChannels)
}

func stereoToMono(stereo []byte) []byte {
	if len(stereo) < 4 {
		return stereo
	}
	mono := make([]byte, len(stereo)/2)
	for i := 0; i+3 < len(stereo); i += 4 {
		l := int16(binary.LittleEndian.Uint16(stereo[i:]))
		r := int16(binary.LittleEndian.Uint16(stereo[i+2:]))
		binary.LittleEndian.PutUint16(mono[i/2:], uint16((int32(l)+int32(r))/2))
	}
	return mono
}
