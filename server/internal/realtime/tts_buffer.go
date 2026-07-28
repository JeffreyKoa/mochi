package realtime

import (
	"strings"
	"unicode/utf8"

	"github.com/mochi-ai/server/internal/config"
)

func isStrongPunctuation(r rune, cfg config.RealtimePipeline) bool {
	return strings.ContainsRune(cfg.TTSStrongPunctuation, r)
}

func isWeakPunctuation(r rune, cfg config.RealtimePipeline) bool {
	return strings.ContainsRune(cfg.TTSWeakPunctuation, r)
}

func isConfiguredPunctuation(r rune, cfg config.RealtimePipeline) bool {
	return strings.ContainsRune(cfg.TTSPunctuation, r)
}

func shouldFlushTTS(buf string, cfg config.RealtimePipeline) bool {
	return shouldFlushTTSEx(buf, cfg, true)
}

func shouldFlushTTSEx(buf string, cfg config.RealtimePipeline, isFirstSegment bool) bool {
	if buf == "" {
		return false
	}
	n := utf8.RuneCountInString(buf)

	if isFirstSegment {
		if n < cfg.TTSFirstMinChars {
			return false
		}
		runes := []rune(buf)
		for _, r := range runes {
			if isConfiguredPunctuation(r, cfg) {
				return true
			}
		}
		return n >= cfg.TTSMinChars*2
	}

	runes := []rune(buf)
	for i, r := range runes {
		count := i + 1
		if isStrongPunctuation(r, cfg) && count >= cfg.TTSMinChars {
			return true
		}
		if isWeakPunctuation(r, cfg) && count >= cfg.TTSWeakPunctMinChars {
			return true
		}
	}

	return n >= cfg.TTSForceFlushChars
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
	min := cfg.TTSMinChars
	if isFirstSegment {
		min = cfg.TTSFirstMinChars
	}

	for i, r := range runes {
		count := i + 1
		if count < min {
			continue
		}
		if isFirstSegment {
			if isConfiguredPunctuation(r, cfg) {
				cut := count
				chunk := string(runes[:cut])
				remaining := string(runes[cut:])
				buf.Reset()
				buf.WriteString(remaining)
				return chunk
			}
		} else {
			if isStrongPunctuation(r, cfg) || (isWeakPunctuation(r, cfg) && count >= cfg.TTSWeakPunctMinChars) {
				cut := count
				chunk := string(runes[:cut])
				remaining := string(runes[cut:])
				buf.Reset()
				buf.WriteString(remaining)
				return chunk
			}
		}
	}

	if len(runes) >= cfg.TTSForceFlushChars {
		for i := len(runes) - 1; i >= 0; i-- {
			if isConfiguredPunctuation(runes[i], cfg) {
				cut := i + 1
				chunk := string(runes[:cut])
				remaining := string(runes[cut:])
				buf.Reset()
				buf.WriteString(remaining)
				return chunk
			}
		}
		chunk := text
		buf.Reset()
		return chunk
	}

	return ""
}
