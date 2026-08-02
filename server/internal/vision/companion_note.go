package vision

import "strings"

// companionVisualReplacements VL 可能带出的技术词 → Mochi 生命体口吻。
var companionVisualReplacements = []struct{ old, new string }{
	{"没拍到", "没看清楚"},
	{"未能拍到", "没看清楚"},
	{"未拍到", "没看清楚"},
	{"未看清", "没看清楚"},
	{"摄像头", "我这边"},
	{"手机镜头", "我眼睛"},
	{"镜头", "眼睛"},
	{"屏幕显示", "我看到"},
	{"屏幕上", "我眼前"},
	{"屏幕", "眼前"},
	{"画面里", "我眼前"},
	{"画面中", "我眼前"},
	{"画面", "眼前"},
	{"照片", "眼前"},
	{"图像", "眼前"},
}

// SanitizeCompanionNote 注入主人对话 Prompt 前，把 VL 可能带出的技术词转为 Mochi 口吻。
// 注意：画面事实来源仍是摄像头（Mochi 唯一视觉输入）；此处只改对用户说话的措辞，不改感知链路。
func SanitizeCompanionNote(note string) string {
	s := strings.TrimSpace(note)
	if s == "" {
		return s
	}
	for _, pair := range companionVisualReplacements {
		s = strings.ReplaceAll(s, pair.old, pair.new)
	}
	return strings.TrimSpace(s)
}
