package realtime

import (
	"strings"
	"unicode/utf8"

	"github.com/mochi-ai/server/internal/config"
)

func shouldFlushTTS(buf string, cfg config.RealtimePipeline) bool {
	if buf == "" {
		return false
	}
	n := utf8.RuneCountInString(buf)
	min := cfg.TTSMinChars
	if min <= 0 {
		min = 5
	}
	if n < min {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(buf)
	for _, p := range cfg.TTSPunctuation {
		if last == p {
			return true
		}
	}
	return n >= min*3
}

func takeFlushSegment(buf *strings.Builder, cfg config.RealtimePipeline) string {
	text := buf.String()
	if !shouldFlushTTS(text, cfg) {
		return ""
	}
	// flush up to last punctuation when possible
	runes := []rune(text)
	cut := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		for _, p := range cfg.TTSPunctuation {
			if runes[i] == p {
				cut = i + 1
				chunk := string(runes[:cut])
				remaining := string(runes[cut:])
				buf.Reset()
				buf.WriteString(remaining)
				return chunk
			}
		}
	}
	chunk := text
	buf.Reset()
	return chunk
}
