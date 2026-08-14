package realtime

import (
	"bytes"
	"encoding/binary"
)

// pcm16ToWAV 将 mono PCM int16 LE 封装为 WAV（供远程 ASR multipart 上传）。
func pcm16ToWAV(pcm []byte, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	dataSize := len(pcm)
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16)) // fmt chunk size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))  // mono
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	byteRate := sampleRate * 2
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))  // block align
	_ = binary.Write(buf, binary.LittleEndian, uint16(16)) // bits
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(pcm)
	return buf.Bytes()
}
