package text

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MoodTag 表示 LLM 输出的语气标记（[mood:xxx]）。
type MoodTag string

const (
	MoodCalm    MoodTag = "calm"
	MoodGentle  MoodTag = "gentle"
	MoodExcited MoodTag = "excited"
	MoodSad     MoodTag = "sad"
	MoodWorried MoodTag = "worried"
	MoodPlayful MoodTag = "playful"
	MoodSerious MoodTag = "serious"
)

var (
	moodTagPattern = `(?:gentle|excited|sad|calm|worried|playful|serious)`
	// 允许 mood 名前后空格，避免 LLM 输出 `[mood: calm]` 时剥离失败
	moodTagRE     = regexp.MustCompile(`(?i)\[mood:\s*` + moodTagPattern + `\s*\]`)
	moodTagHeadRE = regexp.MustCompile(`(?i)^\s*\[mood:\s*(` + moodTagPattern + `)\s*\]\s*`)
	// 清理流式/漏标产生的残缺标记（如 [mood:pla、m]）
	moodTagOrphanRE  = regexp.MustCompile(`(?i)\[mood:[^\]]*\]?`)
	moodStrayTailRE  = regexp.MustCompile(`(?i)^m\]`)
	moodStrayMidRE   = regexp.MustCompile(`([。！？.!?])\s*\]`)
)

// ToneSegment 为剥离 mood 标记后的单句文本。
type ToneSegment struct {
	Mood MoodTag
	Text string
}

// ParseToneSegment 解析句首 [mood:xxx]；无标记时 Mood 为空（由调用方继承上一句）。
func ParseToneSegment(raw string) ToneSegment {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ToneSegment{}
	}
	loc := moodTagHeadRE.FindStringSubmatchIndex(trimmed)
	if loc == nil {
		return ToneSegment{Text: trimmed}
	}
	sub := moodTagHeadRE.FindStringSubmatch(trimmed)
	mood := MoodCalm
	if len(sub) > 1 {
		mood = MoodTag(strings.ToLower(sub[1]))
	}
	text := strings.TrimSpace(trimmed[loc[1]:])
	return ToneSegment{Mood: mood, Text: text}
}

// StripMoodTags 移除全文中的所有 [mood:xxx] 标记（含残缺片段）。
func StripMoodTags(s string) string {
	if s == "" {
		return ""
	}
	out := moodTagRE.ReplaceAllString(s, "")
	out = moodTagOrphanRE.ReplaceAllString(out, "")
	out = moodStrayTailRE.ReplaceAllString(out, "")
	out = moodStrayMidRE.ReplaceAllString(out, "$1")
	return collapseSpaces(strings.TrimSpace(out))
}

// MoodTracker 在流式/分句场景下继承上一句 mood。
type MoodTracker struct {
	current MoodTag
}

// NewMoodTracker 创建 mood 追踪器，首句默认 calm。
func NewMoodTracker() *MoodTracker {
	return &MoodTracker{current: MoodCalm}
}

// NewMoodTrackerWithDefault 创建 mood 追踪器，首句无 [mood:xxx] 时继承 defaultMood（Phase 4 流式默认）。
func NewMoodTrackerWithDefault(defaultMood MoodTag) *MoodTracker {
	if defaultMood == "" {
		defaultMood = MoodCalm
	}
	return &MoodTracker{current: defaultMood}
}

// InferDefaultMood 根据用户情绪上下文推断 TTS 首句默认 mood（LLM 漏标时使用）。
func InferDefaultMood(userMood, intent string, needsEmpathy bool) MoodTag {
	if needsEmpathy || intent == "vent" || userMood == "stressed" || userMood == "sad" {
		return MoodGentle
	}
	if intent == "joke" || userMood == "happy" {
		return MoodPlayful
	}
	return MoodCalm
}

// CountMoodTags 统计回复中的 [mood:xxx] 标记数量。
func CountMoodTags(s string) int {
	if s == "" {
		return 0
	}
	return len(moodTagRE.FindAllString(s, -1))
}

// CountSpeakSentences 估算可分句朗读的句子数（用于 mood tag 遵标率）。
func CountSpeakSentences(s string) int {
	s = StripMoodTags(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	parts := sentenceSplitRE.Split(s, -1)
	n := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

// MoodTagComplianceRate 返回 mood tag 遵标率（标记数 / 句子数，上限 1.0）。
func MoodTagComplianceRate(raw string) float64 {
	sentences := CountSpeakSentences(raw)
	if sentences == 0 {
		return 0
	}
	tags := CountMoodTags(raw)
	rate := float64(tags) / float64(sentences)
	if rate > 1 {
		return 1
	}
	return rate
}

var sentenceSplitRE = regexp.MustCompile(`[。！？.!?\n]+`)

// Process 解析分句并更新继承 mood。
func (mt *MoodTracker) Process(raw string) ToneSegment {
	seg := ParseToneSegment(raw)
	if seg.Mood != "" {
		mt.current = seg.Mood
	} else {
		seg.Mood = mt.current
		if seg.Mood == "" {
			seg.Mood = MoodCalm
		}
	}
	return seg
}

// StreamMoodStripper 从流式 token 中剥离 [mood:xxx]，避免 UI 短暂闪现标记。
type StreamMoodStripper struct {
	hold strings.Builder
}

// Feed 处理增量 token，返回可安全展示给用户的文本。
func (sm *StreamMoodStripper) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	buf := sm.hold.String() + chunk
	sm.hold.Reset()
	return stripStreamingMoodTags(buf, &sm.hold)
}

// Flush 刷出尾部缓冲（可能含未闭合标记的残留，尽量剥离已知 tag）。
func (sm *StreamMoodStripper) Flush() string {
	held := sm.hold.String()
	sm.hold.Reset()
	if held == "" {
		return ""
	}
	return StripMoodTags(held)
}

func stripStreamingMoodTags(s string, hold *strings.Builder) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			rest := s[i:]
			// 任何 [mood:...] 一律整段剥离（含未知 mood 名），避免流式切 token 残留 m]
			if len(rest) >= 6 && strings.EqualFold(rest[:6], "[mood:") {
				end := strings.Index(rest, "]")
				if end < 0 {
					hold.WriteString(rest)
					return out.String()
				}
				i += end + 1
				continue
			}
			end := strings.Index(s[i:], "]")
			if end < 0 {
				hold.WriteString(s[i:])
				return out.String()
			}
			candidate := s[i : i+end+1]
			if moodTagRE.MatchString(candidate) {
				i += end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String()
}
