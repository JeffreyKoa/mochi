package emotion

import (
	"time"

	"github.com/mochi-ai/server/internal/vision"
)

// PerceptionBuffer 单 turn 多通道感知缓冲（V3 察言观色：眼耳言并行汇入）。
type PerceptionBuffer struct {
	Text          string
	TextAt        time.Time
	TextReady     bool
	Acoustic      AcousticHint
	AcousticAt    time.Time
	AcousticReady bool
	Visual        vision.Hint
	VisualAt      time.Time
	VisualReady   bool
}

// MarkText 写入 ASR 文本。
func (b *PerceptionBuffer) MarkText(text string) {
	if b == nil {
		return
	}
	b.Text = text
	b.TextAt = time.Now()
	b.TextReady = true
}

// MarkAcoustic 写入声学情绪。
func (b *PerceptionBuffer) MarkAcoustic(h AcousticHint) {
	if b == nil {
		return
	}
	b.Acoustic = h
	b.AcousticAt = time.Now()
	b.AcousticReady = true
}

// MarkVisual 写入视觉 Hint。
func (b *PerceptionBuffer) MarkVisual(h vision.Hint) {
	if b == nil {
		return
	}
	b.Visual = h
	b.VisualAt = time.Now()
	b.VisualReady = true
}

// Ready 是否三路均已就绪（缺通道视为已就绪）。
func (b *PerceptionBuffer) Ready(requireText, requireAcoustic, requireVisual bool) bool {
	if b == nil {
		return false
	}
	if requireText && !b.TextReady {
		return false
	}
	if requireAcoustic && !b.AcousticReady {
		return false
	}
	if requireVisual && !b.VisualReady {
		return false
	}
	return true
}

// BuildHintFromBuffer 从缓冲融合情绪 Hint（等同 BuildHint 规则）。
func BuildHintFromBuffer(buf *PerceptionBuffer, minAcoustic, minVisual float64) Hint {
	if buf == nil {
		return Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
	}
	text := buf.Text
	if !buf.TextReady {
		text = ""
	}
	acoustic := buf.Acoustic
	if !buf.AcousticReady {
		acoustic = EmptyAcousticHint()
	}
	visual := buf.Visual
	if !buf.VisualReady {
		visual = vision.EmptyHint()
	}
	quick := QuickDetect(text)
	merged := MergeAcousticHint(Hint{}, quick, acoustic, minAcoustic)
	if minVisual <= 0 {
		minVisual = 0.6
	}
	return MergeVisualHint(merged, visual, minVisual)
}

// BuildEarlyHint 无 ASR 文本时，仅从声学+视觉融合（V3b 动画先行）。
func BuildEarlyHint(acoustic AcousticHint, visual vision.Hint, minAcoustic, minVisual float64) Hint {
	neutral := Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
	h := MergeAcousticHint(Hint{}, neutral, acoustic, minAcoustic)
	if minVisual <= 0 {
		minVisual = 0.6
	}
	return MergeVisualHint(h, visual, minVisual)
}

// ShouldEarlyAnimate 是否足够置信触发早推动画。
func ShouldEarlyAnimate(h Hint, minVisualConf float64) bool {
	if h.NeedsEmpathy {
		return true
	}
	switch h.UserMood {
	case "happy":
		return true
	case "stressed", "sad":
		return true
	default:
		return false
	}
}
