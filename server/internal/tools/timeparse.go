package tools

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var loc = time.FixedZone("CST", 8*3600)

var cnDigit = map[rune]int{
	'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
}

// ParseFireAt extracts reminder time from Chinese colloquial text (UTC+8).
func ParseFireAt(text string, now time.Time) (time.Time, bool) {
	if now.IsZero() {
		now = time.Now().In(loc)
	} else {
		now = now.In(loc)
	}
	dayOffset := 0

	switch {
	case strings.Contains(text, "后天"):
		dayOffset = 2
	case strings.Contains(text, "明天"):
		dayOffset = 1
	case strings.Contains(text, "今天"), strings.Contains(text, "今晚"), strings.Contains(text, "今天晚上"):
		dayOffset = 0
	default:
		return time.Time{}, false
	}

	hour, minute, ok := extractHourMinute(text)
	if !ok {
		switch {
		case strings.Contains(text, "凌晨"):
			hour, minute = 0, 0
		case strings.Contains(text, "早") || strings.Contains(text, "上午"):
			hour, minute = 9, 0
		case strings.Contains(text, "中午"):
			hour, minute = 12, 0
		case strings.Contains(text, "下午"):
			hour, minute = 15, 0
		case strings.Contains(text, "晚") || strings.Contains(text, "夜"):
			hour, minute = 20, 0
		default:
			return time.Time{}, false
		}
	} else {
		hour = normalizeHourForDayPart(text, hour)
	}

	fire := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc).AddDate(0, 0, dayOffset)
	if fire.Before(now) && dayOffset == 0 {
		fire = fire.AddDate(0, 0, 1)
	}
	return fire, true
}

func extractHourMinute(text string) (hour, minute int, ok bool) {
	token := `(\d{1,2}|[零一二两三四五六七八九十]+)`

	reHalf := regexp.MustCompile(token + `\s*点\s*半`)
	if m := reHalf.FindStringSubmatch(text); len(m) >= 2 {
		if h, ok := parseHourToken(m[1]); ok {
			return h, 30, true
		}
	}

	re := regexp.MustCompile(token + `\s*[点:：时]\s*(\d{1,2}|[零一二两三四五六七八九十]+)?`)
	if m := re.FindStringSubmatch(text); len(m) >= 2 {
		h, okH := parseHourToken(m[1])
		if !okH || h < 0 || h > 23 {
			return 0, 0, false
		}
		min := 0
		if len(m) >= 3 && m[2] != "" {
			if n, okM := parseHourToken(m[2]); okM && n >= 0 && n <= 59 {
				min = n
			}
		}
		return h, min, true
	}

	re2 := regexp.MustCompile(token + `\s*点`)
	if m2 := re2.FindStringSubmatch(text); len(m2) >= 2 {
		if h, ok := parseHourToken(m2[1]); ok && h >= 0 && h <= 23 {
			return h, 0, true
		}
	}
	return 0, 0, false
}

func normalizeHourForDayPart(text string, hour int) int {
	isMorning := strings.Contains(text, "早上") || strings.Contains(text, "上午") || strings.Contains(text, "凌晨")
	isAfternoon := strings.Contains(text, "下午")
	isEvening := strings.Contains(text, "晚上") || strings.Contains(text, "今晚") || strings.Contains(text, "夜里") || strings.Contains(text, "夜间")
	if isEvening && !isMorning {
		if hour >= 1 && hour <= 11 {
			return hour + 12
		}
	}
	if isAfternoon && hour >= 1 && hour <= 11 {
		return hour + 12
	}
	return hour
}

func parseHourToken(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	runes := []rune(s)
	if len(runes) == 1 {
		if v, ok := cnDigit[runes[0]]; ok {
			return v, true
		}
	}
	if strings.Contains(s, "十") {
		if s == "十" {
			return 10, true
		}
		if strings.HasPrefix(s, "十") {
		 tail := strings.TrimPrefix(s, "十")
			if tail == "" {
				return 10, true
			}
			if v, ok := cnDigit[[]rune(tail)[0]]; ok {
				return 10 + v, true
			}
		}
		if strings.HasSuffix(s, "十") {
			head := strings.TrimSuffix(s, "十")
			if v, ok := cnDigit[[]rune(head)[0]]; ok {
				return v * 10, true
			}
		}
		idx := strings.Index(s, "十")
		if idx > 0 && idx < len(s)-1 {
			head := []rune(s[:idx])
			tail := []rune(s[idx+1:])
			h := cnDigit[head[0]]
			m := cnDigit[tail[0]]
			return h*10 + m, true
		}
	}
	return 0, false
}

func ExtractReminderTitle(text string) string {
	text = strings.TrimSpace(text)
	for _, suffix := range []string{
		"，帮我记一下", "帮我记一下", "，到时候提醒我", "到时候提醒我",
		"，提醒我", "提醒我", "记得", "别忘了", "记一下",
	} {
		if idx := strings.Index(text, suffix); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
		}
	}
	text = trimTimePhrases(text)
	text = regexp.MustCompile(`^(明天|后天|今天|今晚|今天晚上|早上|上午|下午|晚上|凌晨)+`).ReplaceAllString(text, "")
	text = strings.Trim(text, "，。！？ \t")
	if text != "" {
		return text
	}
	return trimTimePhrases(text)
}

func ExtractTodoTitle(text string) string {
	text = strings.TrimSpace(text)
	re := regexp.MustCompile(`帮(?:我)?(?:把)?(.+?)(?:记下来|记下|记一下)?[。！？\s]*$`)
	if m := re.FindStringSubmatch(text); len(m) >= 2 {
		if t := strings.TrimSpace(m[1]); t != "" {
			return t
		}
	}
	lower := strings.ToLower(text)
	for _, prefix := range []string{"帮我记", "记一下", "记下来", "记下", "待办", "todo", "记个", "帮我把"} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(text[idx+len(prefix):])
			rest = strings.Trim(rest, "：:一下")
			rest = strings.TrimSuffix(rest, "记下来")
			rest = strings.TrimSuffix(rest, "记下")
			rest = strings.TrimSpace(rest)
			if rest != "" {
				return rest
			}
		}
	}
	return text
}

func trimTimePhrases(s string) string {
	s = regexp.MustCompile(`(明天|后天|今天|今晚|今天晚上|早上|上午|中午|下午|晚上|凌晨)?\s*(\d{1,2}|[零一二两三四五六七八九十]+)\s*[点:：时]\s*(半|\d{1,2}|[零一二两三四五六七八九十]+)?\s*(分|钟)?`).ReplaceAllString(s, "")
	s = strings.Trim(s, "，。！？ \t")
	return strings.TrimSpace(s)
}

func FormatFireAt(t time.Time) string {
	return t.In(loc).Format("1月2日 15:04")
}
