package text

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	fullWidthParenRE = regexp.MustCompile(`（[^）]*）`)
	halfWidthParenRE = regexp.MustCompile(`\([^)]*\)`)
	asteriskActionRE = regexp.MustCompile(`\*[^*]+\*`)
	emojiRE          = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)
)

// SanitizeSpokenReply strips stage directions, emojis, and unwanted formatting.
func SanitizeSpokenReply(s string) string {
	s = StripActionParentheticals(s)
	s = emojiRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "光粒", "Mochi")
	return collapseSpaces(strings.TrimSpace(s))
}

// StripActionParentheticals removes stage-direction text wrapped in parentheses or asterisks.
func StripActionParentheticals(s string) string {
	prev := ""
	for prev != s {
		prev = s
		s = fullWidthParenRE.ReplaceAllString(s, "")
		s = halfWidthParenRE.ReplaceAllString(s, "")
		s = asteriskActionRE.ReplaceAllString(s, "")
	}
	return collapseSpaces(strings.TrimSpace(s))
}

// StreamSanitizer strips action parentheses from streaming LLM tokens.
type StreamSanitizer struct {
	hold strings.Builder
}

func (ss *StreamSanitizer) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	buf := ss.hold.String() + chunk
	ss.hold.Reset()
	return stripCompleteParentheticals(buf, &ss.hold)
}

// Flush returns any trailing text not held inside an unclosed parenthetical.
func (ss *StreamSanitizer) Flush() string {
	held := ss.hold.String()
	ss.hold.Reset()
	if held == "" {
		return ""
	}
	h := strings.TrimLeft(held, "（(*")
	return collapseSpaces(strings.TrimSpace(h))
}

func stripCompleteParentheticals(s string, hold *strings.Builder) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '（':
			if end := strings.Index(s[i+size:], "）"); end >= 0 {
				i += size + end + len("）")
				continue
			}
			hold.WriteString(s[i:])
			return collapseSpaces(out.String())
		case '(':
			if end := strings.Index(s[i+size:], ")"); end >= 0 {
				i += size + end + 1
				continue
			}
			hold.WriteString(s[i:])
			return collapseSpaces(out.String())
		case '*':
			if end := strings.Index(s[i+size:], "*"); end >= 0 {
				i += size + end + len("*")
				continue
			}
			hold.WriteString(s[i:])
			return collapseSpaces(out.String())
		default:
			out.WriteRune(r)
			i += size
		}
	}
	return collapseSpaces(out.String())
}

func collapseSpaces(s string) string {
	if s == "" {
		return ""
	}
	var buf strings.Builder
	inSpace := false
	var prevRune rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inSpace = true
			continue
		}
		if inSpace {
			if buf.Len() > 0 && !isCJK(prevRune) && !isCJK(r) {
				buf.WriteByte(' ')
			}
			inSpace = false
		}
		buf.WriteRune(r)
		prevRune = r
	}
	return buf.String()
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3000 && r <= 0x303F) ||
		(r >= 0xFF00 && r <= 0xFFEF)
}

