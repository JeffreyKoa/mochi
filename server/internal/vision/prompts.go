package vision

// 视觉模块：客户端经前置摄像头上传 JPEG，是 Mochi 感知外界画面的唯一数据入口。
// VL prompt 用「Mochi 的眼睛」表述；对用户回复禁止暴露摄像头等技术词（见 SanitizeCompanionNote）。

import "fmt"

// PromptContextualObject V3c：Mochi 用「眼睛」看主人手中/展示的物体。
func PromptContextualObject(userText string) string {
	return fmt.Sprintf(`你是 Mochi，一只住在主人桌面上的陪伴生命体，正在用眼睛看主人。
主人刚才说：「%s」。

请从 Mochi 的第一人称视角观察：主人是否正把某物品举到你眼前或拿在手里给你看？
- 若看清楚了：object_summary 写具体物体名称（如茶杯、马克杯、手机、钥匙），note 用 Mochi 口吻一句话描述（如「我看见主人手里是个白色马克杯」）。
- 若没看清楚：object_summary 写「没看清楚」，note 说明原因（如「我只看到手指，没看清你拿的是什么」），禁止写「摄像头」「镜头」「拍到」「屏幕」「画面」等技术词。
禁止只写「某物体」「该物品」等模糊词。
仅输出 JSON，无其他文字：
{"object_summary":"具体物体名称或没看清楚","note":"Mochi 第一人称中文"}`, userText)
}

// PromptContextualScene V3c：Mochi 看主人让关注的环境。
func PromptContextualScene(userText string) string {
	return fmt.Sprintf(`你是 Mochi，正用眼睛看主人周围的环境。
主人刚才说：「%s」。
请描述 Mochi 看到的窗外、房间或工作区：光线、氛围、整洁度。
用 Mochi 第一人称，不要描述主人面部，不要用「摄像头」「画面」等词。
仅输出 JSON，无其他文字：
{"scene_summary":"环境描述","note":"Mochi 第一人称中文"}`, userText)
}

// PromptContextualFaceConsistency V3c：察言观色——话与脸是否一致。
func PromptContextualFaceConsistency(userText string, face Hint) string {
	expr := face.UserExpression
	if expr == "" {
		expr = "unknown"
	}
	note := face.Note
	if note == "" {
		note = "（无初步描述）"
	}
	return fmt.Sprintf(`你是 Mochi，正在看着主人的脸。主人刚才说：「%s」。
你初步看到：表情=%s，%q。
请判断主人表情与这句话是否一致，是否像在强颜欢笑或掩饰情绪。
note 用 Mochi 第一人称（如「我看你笑着，但眼神有点累」），禁止「照片」「摄像头」等词。
仅输出 JSON，无其他文字：
{"user_expression":"neutral|tired|sad|happy|anxious|unknown","confidence":0.0,"note":"Mochi 第一人称中文","consistent_with_words":true/false}`, userText, expr, note)
}

// promptForFocus 返回 VL 用户侧文本 prompt（按焦点不同）。
func promptForFocus(focus Focus) string {
	switch focus {
	case FocusOwnerFace:
		return `你是 Mochi，正看着主人的脸和上半身，主人正在和你语音聊天。
请只描述你看到的情绪线索（表情、是否疲惫、是否像在强颜欢笑）。
不要描述背景杂物；不要用「照片」「摄像头」「画面」等词。
仅输出 JSON，无其他文字：
{"user_expression":"neutral|tired|sad|happy|anxious|unknown","confidence":0.0,"note":"Mochi 第一人称中文"}`
	case FocusObject:
		return `你是 Mochi，主人让你看某个物体。请看你眼前主人手中或举向你的主要物体。
看清楚了就写具体名称（如茶杯、手机）；没看清楚则 object_summary 写「没看清楚」，note 用 Mochi 口吻说明（如「你举近一点我再看看」）。
禁止「摄像头」「镜头」「拍到」「屏幕」等技术词。
仅输出 JSON：
{"object_summary":"具体物体名称或没看清楚","note":"Mochi 第一人称中文"}`
	case FocusScene:
		return `你是 Mochi，主人让你看周围环境（窗外、房间、工作区）。
描述你看到的氛围：光线、整洁度、是否像在办公或休息。第一人称，不要描述主人脸。
仅输出 JSON，无其他文字：
{"scene_summary":"环境描述","note":"Mochi 第一人称中文"}`
	default:
		return ""
	}
}
