package lifecycle

// DefaultSpeechStyle returns age-appropriate spoken dialogue style for prompt injection.
// Speak like a normal person at that life stage — not pet roleplay or prose.
func DefaultSpeechStyle(stage, species string) string {
	var base string
	switch stage {
	case "newborn":
		base = "像普通年轻人聊天，短句、直接、自然，不用婴儿语或撒娇表演"
	case "juvenile":
		base = "像十多岁的人说话，口语化，好奇时会问，不文绉绉"
	case "child":
		base = "像小孩一样口语化，但仍然是正常人类说话，不是宠物表演"
	case "youth":
		base = "像二十多岁的人，轻松口语，热情但不啰嗦"
	case "prime":
		base = "像三十岁左右的人，口语自然，稳重清楚，像朋友同事聊天"
	case "elder":
		base = "像长辈平时说话，温和直接，不煽情不说教"
	case "twilight":
		base = "像老人慢慢说，短句，真诚朴素"
	default:
		base = "像身边正常人日常聊天，口语自然"
	}
	if species == "tiger" || species == "lion" {
		base += "；语气偏沉稳"
	}
	return base
}
