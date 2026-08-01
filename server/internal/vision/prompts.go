package vision

// promptForFocus 返回 VL 用户侧文本 prompt（按焦点不同）。
func promptForFocus(focus Focus) string {
	switch focus {
	case FocusOwnerFace:
		return `这是一张主人正在和桌面宠物 Mochi 语音聊天的照片。
请只描述主人面部与上半身可见的情绪线索（表情、是否疲惫、是否像在强颜欢笑）。
不要描述背景、屏幕、桌面物品或房间杂物。
仅输出 JSON，无其他文字：
{"user_expression":"neutral|tired|sad|happy|anxious|unknown","confidence":0.0,"note":"一句话中文描述"}`
	case FocusObject:
		return `主人想让 Mochi 看某个物体。请描述画面中心或主人展示的主要物体是什么。
不要描述背景杂物或主人面部细节。
仅输出 JSON：
{"object_summary":"物体描述","note":"一句话中文"}`
	case FocusScene:
		return `主人想让 Mochi 看周围环境（窗外、房间、工作区）。
请描述环境氛围：光线明暗、天气感、整洁度、是否像在办公或休息。
不要描述主人面部细节，不要罗列桌面小物件。
仅输出 JSON，无其他文字：
{"scene_summary":"环境描述","note":"一句话中文"}`
	default:
		return ""
	}
}
