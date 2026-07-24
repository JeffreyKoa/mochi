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
)

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
	// Drop an unfinished action block rather than speaking/displaying it.
	return collapseSpaces(strings.TrimSpace(stripCompleteParentheticals(held, &ss.hold)))
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
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
