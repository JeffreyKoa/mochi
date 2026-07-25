package realtime

import (
	"strings"
	"unicode/utf8"

	"github.com/mochi-ai/server/internal/config"
)

const strongPunctuation = "。！？~!?.;\n"
const weakPunctuation = "，、,"

func isStrongPunctuation(r rune) bool {
	return strings.ContainsRune(strongPunctuation, r)
}

func shouldFlushTTS(buf string, cfg config.RealtimePipeline) bool {
	return shouldFlushTTSEx(buf, cfg, true)
}

func shouldFlushTTSEx(buf string, cfg config.RealtimePipeline, isFirstSegment bool) bool {
	if buf == "" {
		return false
	}
	n := utf8.RuneCountInString(buf)
	min := cfg.TTSMinChars
	if min <= 0 {
		min = 5
	}

	// For first segment: flush quickly (4+ chars) on any punctuation for fast TTFT
	if isFirstSegment {
		if n < 4 {
			return false
		}
		last, _ := utf8.DecodeLastRuneInString(buf)
		for _, p := range cfg.TTSPunctuation {
			if last == p {
				return true
			}
		}
		return n >= min*2
	}

	// For continuation segments: prevent choppy breaks on weak punctuation
	last, _ := utf8.DecodeLastRuneInString(buf)
	if isStrongPunctuation(last) && n >= min {
		return true
	}
	// Weak punctuation (e.g. comma) requires larger character buffer before flush
	if strings.ContainsRune(weakPunctuation, last) && n >= 14 {
		return true
	}

	return n >= min*3
}

func takeFlushSegment(buf *strings.Builder, cfg config.RealtimePipeline) string {
	return takeFlushSegmentEx(buf, cfg, true)
}

func takeFlushSegmentEx(buf *strings.Builder, cfg config.RealtimePipeline, isFirstSegment bool) string {
	text := buf.String()
	if !shouldFlushTTSEx(text, cfg, isFirstSegment) {
		return ""
	}

	runes := []rune(text)

	// For continuation segments: prioritize cutting at strong punctuation
	if !isFirstSegment {
		for i := len(runes) - 1; i >= 0; i-- {
			if isStrongPunctuation(runes[i]) {
				cut := i + 1
				chunk := string(runes[:cut])
				remaining := string(runes[cut:])
				buf.Reset()
				buf.WriteString(remaining)
				return chunk
			}
		}
	}

	// Cut at any configured punctuation
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		for _, p := range cfg.TTSPunctuation {
			if r == p {
				cut := i + 1
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
