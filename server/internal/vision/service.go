package vision

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/config"
)

// Service 调用 DashScope Qwen-VL，按焦点生成 VisualHint。
type Service struct {
	cfg    config.VisionConfig
	vl     *vlClient
	aiBase string
	aiKey  string
}

// NewService 创建视觉服务；未启用时 Describe 直接返回 EmptyHint。
func NewService(app config.AIConfig, vis config.VisionConfig) *Service {
	timeout := time.Duration(vis.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	model := vis.Model
	if model == "" {
		model = "qwen-vl-plus"
	}
	return &Service{
		cfg:    vis,
		vl:     newVLClient(app.APIBase, app.APIKey, model, timeout),
		aiBase: app.APIBase,
		aiKey:  app.APIKey,
	}
}

// Enabled 服务端 vision 总开关。
func (s *Service) Enabled() bool {
	return s != nil && s.cfg.Enabled && s.aiKey != ""
}

// Describe 分析 JPEG 帧；sessionID 仅用于日志。
func (s *Service) Describe(ctx context.Context, jpeg []byte, focus Focus, sessionID string) Hint {
	start := time.Now()
	if s == nil || !s.Enabled() {
		log.Printf("[vision] session=%s skip reason=service_disabled elapsed_ms=%d", sessionID, time.Since(start).Milliseconds())
		return EmptyHint()
	}
	if focus == FocusSkip || len(jpeg) == 0 {
		reason := "focus_skip"
		if len(jpeg) == 0 {
			reason = "empty_jpeg"
		}
		log.Printf("[vision] session=%s skip reason=%s jpeg_bytes=0 elapsed_ms=%d", sessionID, reason, time.Since(start).Milliseconds())
		return Hint{Focus: FocusSkip, Skipped: true, SkipReason: reason}
	}

	prompt := promptForFocus(focus)
	if prompt == "" {
		log.Printf("[vision] session=%s skip reason=unknown_focus focus=%s elapsed_ms=%d", sessionID, focus, time.Since(start).Milliseconds())
		return Hint{Focus: FocusSkip, Skipped: true, SkipReason: "unknown_focus"}
	}

	log.Printf("[vision] session=%s vl_start focus=%s jpeg_bytes=%d model=%s", sessionID, focus, len(jpeg), s.cfg.Model)
	raw, err := s.vl.chat(ctx, jpeg, prompt)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[vision] session=%s vl_error focus=%s err=%v elapsed_ms=%d", sessionID, focus, err, elapsed)
		return Hint{Focus: focus, Skipped: true, SkipReason: "vl_error: " + err.Error()}
	}

	hint := parseVLResponse(focus, raw)
	hint.Focus = focus
	switch focus {
	case FocusOwnerFace:
		log.Printf("[vision] session=%s vl_ok focus=%s expression=%s conf=%.2f note=%q elapsed_ms=%d raw_len=%d",
			sessionID, focus, hint.UserExpression, hint.ExpressionConfidence, truncate(hint.Note, 60), elapsed, len(raw))
	case FocusObject:
		log.Printf("[vision] session=%s vl_ok focus=%s object=%q note=%q elapsed_ms=%d raw_len=%d",
			sessionID, focus, truncate(hint.ObjectSummary, 60), truncate(hint.Note, 60), elapsed, len(raw))
	case FocusScene:
		log.Printf("[vision] session=%s vl_ok focus=%s scene=%q note=%q elapsed_ms=%d raw_len=%d",
			sessionID, focus, truncate(hint.SceneSummary, 60), truncate(hint.Note, 60), elapsed, len(raw))
	default:
		log.Printf("[vision] session=%s vl_ok focus=%s note=%q elapsed_ms=%d raw_len=%d",
			sessionID, focus, truncate(hint.Note, 60), elapsed, len(raw))
	}
	return hint
}

// BuildRouteInput 构造路由输入（含配置化触发词）。
func (s *Service) BuildRouteInput(userText, intent string, isVoiceTurn, hasFrame bool) RouteInput {
	return RouteInput{
		UserText:      userText,
		Intent:        intent,
		IsVoiceTurn:   isVoiceTurn,
		VisionEnabled: s != nil && s.Enabled(),
		HasFrame:      hasFrame,
		ObjectKeys:    s.cfg.ObjectTriggerKeywords,
		SceneKeys:     s.cfg.SceneTriggerKeywords,
	}
}

// RouteAndDescribe 路由 + 分析合一（便于 pipeline 调用）。
func (s *Service) RouteAndDescribe(ctx context.Context, jpeg []byte, in RouteInput, sessionID string) Hint {
	focus := RouteFocus(in)
	if focus == FocusSkip {
		return Hint{Focus: FocusSkip, Skipped: true, SkipReason: "router_skip"}
	}
	return s.Describe(ctx, jpeg, focus, sessionID)
}

type vlFaceJSON struct {
	UserExpression string  `json:"user_expression"`
	Confidence     float64 `json:"confidence"`
	Note           string  `json:"note"`
}

type vlObjectJSON struct {
	ObjectSummary string `json:"object_summary"`
	Note          string `json:"note"`
}

type vlSceneJSON struct {
	SceneSummary string `json:"scene_summary"`
	Note         string `json:"note"`
}

func parseVLResponse(focus Focus, raw string) Hint {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var hint Hint
	switch focus {
	case FocusOwnerFace:
		var j vlFaceJSON
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			log.Printf("[vision][parse] owner_face json_fail err=%v raw=%q", err, truncate(raw, 120))
			hint.Note = raw
			hint.UserExpression = "unknown"
			return hint
		}
		hint.UserExpression = normalizeExpression(j.UserExpression)
		hint.ExpressionConfidence = j.Confidence
		hint.Note = strings.TrimSpace(j.Note)
	case FocusObject:
		var j vlObjectJSON
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			log.Printf("[vision][parse] object json_fail err=%v raw=%q", err, truncate(raw, 120))
			hint.Note = raw
			return hint
		}
		hint.ObjectSummary = strings.TrimSpace(j.ObjectSummary)
		hint.Note = strings.TrimSpace(j.Note)
		if hint.Note == "" && hint.ObjectSummary != "" {
			hint.Note = hint.ObjectSummary
		}
	case FocusScene:
		var j vlSceneJSON
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			log.Printf("[vision][parse] scene json_fail err=%v raw=%q", err, truncate(raw, 120))
			hint.Note = raw
			return hint
		}
		hint.SceneSummary = strings.TrimSpace(j.SceneSummary)
		hint.Note = strings.TrimSpace(j.Note)
		if hint.Note == "" && hint.SceneSummary != "" {
			hint.Note = hint.SceneSummary
		}
	default:
		hint.Note = raw
	}
	if focus == FocusOwnerFace && hint.Note == "" && hint.UserExpression != "" && hint.UserExpression != "unknown" {
		hint.Note = "主人看起来" + expressionLabel(hint.UserExpression)
	}
	return hint
}

func normalizeExpression(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "neutral", "tired", "sad", "happy", "anxious":
		return s
	default:
		return "unknown"
	}
}

func expressionLabel(expr string) string {
	switch expr {
	case "tired":
		return "疲惫"
	case "sad":
		return "难过"
	case "happy":
		return "开心"
	case "anxious":
		return "紧张"
	case "neutral":
		return "平静"
	default:
		return "情绪不明"
	}
}
