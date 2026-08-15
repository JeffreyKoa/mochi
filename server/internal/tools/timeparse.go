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

// ParseRelativeFireAt handles colloquial relative times like "两分钟后".
func ParseRelativeFireAt(text string, now time.Time) (time.Time, bool) {
	if now.IsZero() {
		now = time.Now().In(loc)
	} else {
		now = now.In(loc)
	}
	text = strings.TrimSpace(text)

	reMin := regexp.MustCompile(`([零一二两三四五六七八九十\d]+)\s*分钟(?:钟)?后`)
	if m := reMin.FindStringSubmatch(text); len(m) >= 2 {
		if mins, ok := parseHourToken(m[1]); ok && mins > 0 && mins <= 24*60 {
			return now.Add(time.Duration(mins) * time.Minute), true
		}
	}
	if strings.Contains(text, "半小时后") {
		return now.Add(30 * time.Minute), true
	}
	if strings.Contains(text, "小时后") || strings.Contains(text, "钟头后") {
		reHr := regexp.MustCompile(`([零一二两三四五六七八九十\d]+)\s*(?:个?\s*)?(?:小时|钟头)后`)
		if m := reHr.FindStringSubmatch(text); len(m) >= 2 {
			if hrs, ok := parseHourToken(m[1]); ok && hrs > 0 && hrs <= 48 {
				return now.Add(time.Duration(hrs) * time.Hour), true
			}
		}
	}
	return time.Time{}, false
}

// ParseFireAt extracts reminder time from Chinese colloquial text (UTC+8).
func ParseFireAt(text string, now time.Time) (time.Time, bool) {
	if t, ok := ParseRelativeFireAt(text, now); ok {
		return t, true
	}
	if now.IsZero() {
		now = time.Now().In(loc)
	} else {
		now = now.In(loc)
	}
	dayOffset := 0
	hasWeekday := false

	switch {
	case strings.Contains(text, "大后天"):
		dayOffset = 3
	case strings.Contains(text, "后天"):
		dayOffset = 2
	case strings.Contains(text, "明天"):
		dayOffset = 1
	case strings.Contains(text, "今天"), strings.Contains(text, "今晚"), strings.Contains(text, "今天晚上"):
		dayOffset = 0
	default:
		// 解析周几 / 星期几 / 礼拜几 (如: "周五", "下周一", "星期三")
		weekdayOffset, matched := parseWeekdayOffset(text, now)
		if matched {
			dayOffset = weekdayOffset
			hasWeekday = true
		} else {
			return time.Time{}, false
		}
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
	if fire.Before(now) && dayOffset == 0 && !hasWeekday {
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

var advanceMinRe = regexp.MustCompile(`提前\s*([零一二两三四五六七八九十\d]+)\s*分钟`)

// parseAdvanceMinutes extracts "提前N分钟" offset from reminder text.
func parseAdvanceMinutes(text string) time.Duration {
	m := advanceMinRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	if n, ok := parseHourToken(m[1]); ok && n > 0 {
		return time.Duration(n) * time.Minute
	}
	return 0
}

// ParseScheduledTime parses colloquial schedule text including bare "下午4点20" and advance offsets.
func ParseScheduledTime(text string) (time.Time, bool) {
	now := time.Now().In(loc)
	advance := parseAdvanceMinutes(text)

	if t, ok := ParseFireAt(text, now); ok {
		return t.Add(-advance), true
	}
	if t, ok := ParseRelativeFireAt(text, now); ok {
		return t.Add(-advance), true
	}
	if h, m, ok := extractHourMinute(text); ok {
		h = normalizeHourForDayPart(text, h)
		fire := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
		if fire.Before(now) {
			fire = fire.AddDate(0, 0, 1)
		}
		return fire.Add(-advance), true
	}
	return time.Time{}, false
}

func inferReminderTitle(text string) string {
	title := ExtractReminderTitle(text)
	if title != "" && len([]rune(title)) >= 2 {
		return title
	}
	switch {
	case strings.Contains(text, "开会"), strings.Contains(text, "会议"):
		return "开会"
	default:
		return "提醒"
	}
}

// parseWeekdayOffset 解析 "周X", "星期X", "礼拜X", "下周X", "下下周X" 相对于当前日期的天数偏移
func parseWeekdayOffset(text string, now time.Time) (int, bool) {
	re := regexp.MustCompile(`(这|本|下下|下)?(?:个)?\s*(?:周|星期|礼拜)\s*([一二三四五六日天\d])`)
	m := re.FindStringSubmatch(text)
	if len(m) < 3 {
		return 0, false
	}

	prefix := m[1]
	dayToken := m[2]

	var targetWeekday int
	switch dayToken {
	case "1", "一":
		targetWeekday = 1
	case "2", "二":
		targetWeekday = 2
	case "3", "三":
		targetWeekday = 3
	case "4", "四":
		targetWeekday = 4
	case "5", "五":
		targetWeekday = 5
	case "6", "六":
		targetWeekday = 6
	case "7", "日", "天":
		targetWeekday = 0
	default:
		return 0, false
	}

	// 统一转为周一=1 ... 周日=7 便于自然周跨周计算
	curr := int(now.Weekday())
	if curr == 0 {
		curr = 7
	}
	tgt := targetWeekday
	if tgt == 0 {
		tgt = 7
	}

	diff := tgt - curr

	switch prefix {
	case "下下":
		if diff <= 0 {
			return diff + 14, true
		}
		return diff + 14, true
	case "下":
		if diff <= 0 {
			// 本周目标日已过（如今天周四，说下周一），下一个周一就是下周一
			return diff + 7, true
		}
		// 本周目标日还没到（如今天周二，说下周五），下周五应为本周五+7天
		return diff + 7, true
	default: // "这", "本", 或无前缀 (如 "周五", "这周五")
		if diff < 0 {
			// 本周该日已过（如今天周四，说"周一"），默认指下一个周一
			return diff + 7, true
		}
		return diff, true
	}
}

