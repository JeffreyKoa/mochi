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

// DescribeOwnerFace 并行感知路径：不依赖 ASR 文本，直接分析主人表情（V3a）。
func (s *Service) DescribeOwnerFace(ctx context.Context, jpeg []byte, sessionID string) Hint {
	if s == nil || !s.Enabled() || len(jpeg) == 0 {
		return EmptyHint()
	}
	log.Printf("[vision] session=%s owner_face_parallel jpeg_bytes=%d", sessionID, len(jpeg))
	return s.Describe(ctx, jpeg, FocusOwnerFace, sessionID)
}

// RefineAfterASR ASR 完成后按需二次 VL；V3c 开启时走 RefineContextual。
func (s *Service) RefineAfterASR(ctx context.Context, jpeg []byte, userText string, faceHint Hint, sessionID string) Hint {
	if s == nil || !s.Enabled() || len(jpeg) == 0 {
		return faceHint
	}
	if s.ContextualPlannerEnabled() {
		return faceHint
	}
	plan := PlanRefine(userText, s.cfg.ObjectTriggerKeywords, s.cfg.SceneTriggerKeywords)
	if !plan.NeedSecondVL {
		return faceHint
	}
	start := time.Now()
	h := s.Describe(ctx, jpeg, plan.Focus, sessionID)
	log.Printf("[vision] session=%s refine_ok focus=%s reason=%s elapsed_ms=%d",
		sessionID, plan.Focus, plan.Reason, time.Since(start).Milliseconds())
	return h
}

// RefineContextual V3c：语义分类驱动 contextual VL。
func (s *Service) RefineContextual(ctx context.Context, jpeg []byte, userText string, faceHint Hint, visualTask string, faceTextClash bool, sessionID string) Hint {
	if s == nil || !s.Enabled() || len(jpeg) == 0 {
		return faceHint
	}
	plan := PlanContextual(userText, faceHint, visualTask, faceTextClash)
	if !plan.NeedSecondVL {
		// V3c 分类超时/返回 none 时，再用 config 关键词兜底
		kwPlan := PlanRefine(userText, s.cfg.ObjectTriggerKeywords, s.cfg.SceneTriggerKeywords)
		if kwPlan.NeedSecondVL {
			plan = kwPlan
			if plan.Focus == FocusObject {
				plan.DynamicPrompt = PromptContextualObject(userText)
			} else if plan.Focus == FocusScene {
				plan.DynamicPrompt = PromptContextualScene(userText)
			}
		}
	}
	if !plan.NeedSecondVL {
		return faceHint
	}
	start := time.Now()
	h := s.DescribeContextual(ctx, jpeg, plan.Focus, plan.DynamicPrompt, sessionID)
	log.Printf("[vision] session=%s refine_contextual focus=%s reason=%s elapsed_ms=%d",
		sessionID, plan.Focus, plan.Reason, time.Since(start).Milliseconds())
	if plan.Focus == FocusOwnerFace && faceHint.IsUsable() {
		return mergeOwnerFaceHints(faceHint, h)
	}
	return h
}

// DescribeContextual 使用动态 prompt 调用 VL（V3c）。
func (s *Service) DescribeContextual(ctx context.Context, jpeg []byte, focus Focus, userPrompt, sessionID string) Hint {
	start := time.Now()
	if s == nil || !s.Enabled() || len(jpeg) == 0 || userPrompt == "" {
		return EmptyHint()
	}
	log.Printf("[vision] session=%s vl_contextual focus=%s jpeg_bytes=%d model=%s",
		sessionID, focus, len(jpeg), s.cfg.Model)
	raw, err := s.vl.chat(ctx, jpeg, userPrompt)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[vision] session=%s vl_contextual_error focus=%s err=%v elapsed_ms=%d",
			sessionID, focus, err, elapsed)
		return Hint{Focus: focus, Skipped: true, SkipReason: "vl_error: " + err.Error()}
	}
	hint := parseVLResponse(focus, raw)
	hint.Focus = focus
	log.Printf("[vision] session=%s vl_contextual_ok focus=%s elapsed_ms=%d note=%q",
		sessionID, focus, elapsed, truncate(hint.Note, 60))
	return hint
}

// ContextualPlannerEnabled V3c 总开关。
func (s *Service) ContextualPlannerEnabled() bool {
	return s != nil && s.Enabled() && s.cfg.ContextualPlanner
}

// ClassifyEnabledConfigured 配置是否启用同步语义分类。
func (s *Service) ClassifyEnabledConfigured() bool {
	return s != nil && s.cfg.ClassifyEnabled
}

func mergeOwnerFaceHints(base, refined Hint) Hint {
	if !refined.IsUsable() {
		return base
	}
	out := base
	if refined.ExpressionConfidence >= base.ExpressionConfidence {
		out.UserExpression = refined.UserExpression
		out.ExpressionConfidence = refined.ExpressionConfidence
	}
	if refined.Note != "" {
		out.Note = refined.Note
	}
	out.Focus = FocusOwnerFace
	out.Skipped = false
	return out
}

// PrefetchOnFrame 是否在 vision_frame 到达时预分析。
func (s *Service) PrefetchOnFrame() bool {
	return s != nil && s.Enabled() && s.cfg.PrefetchOnFrame
}

// ParallelOwnerFace V3a 默认并行；sequential_owner_face=true 时关闭。
func (s *Service) ParallelOwnerFace() bool {
	return s != nil && s.Enabled() && !s.cfg.SequentialOwnerFace
}

// EarlyAnimationEnabled V3b 感知先到先推动画。
func (s *Service) EarlyAnimationEnabled() bool {
	return s != nil && s.Enabled() && s.cfg.EarlyAnimation
}

// EarlyAnimationMinConf 早推动画视觉置信下限。
func (s *Service) EarlyAnimationMinConf() float64 {
	if s == nil || s.cfg.EarlyAnimationMinConf <= 0 {
		return 0.65
	}
	return s.cfg.EarlyAnimationMinConf
}

// MinVisualConf 视觉融合/早推共用阈值。
func (s *Service) MinVisualConf() float64 {
	if s == nil || s.cfg.MinExpressionConfidence <= 0 {
		return 0.6
	}
	return s.cfg.MinExpressionConfidence
}

// WaitTimeout VL 等待/HTTP 超时。
func (s *Service) WaitTimeout() time.Duration {
	if s == nil || s.cfg.TimeoutMS <= 0 {
		return 5 * time.Second
	}
	return time.Duration(s.cfg.TimeoutMS) * time.Millisecond
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
