package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

type mockStreamAI struct {
	name         string
	chunks       []ai.ChatChunk
	lastMessages []ai.Message
}

func (m *mockStreamAI) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock-model"
}

func (m *mockStreamAI) Chat(_ context.Context, _ ai.ChatRequest) (*ai.ChatResponse, error) {
	return &ai.ChatResponse{Content: "mock"}, nil
}

func (m *mockStreamAI) ChatStream(_ context.Context, req ai.ChatRequest) (<-chan ai.ChatChunk, error) {
	m.lastMessages = req.Messages
	ch := make(chan ai.ChatChunk, len(m.chunks)+1)
	go func() {
		defer close(ch)
		for _, chunk := range m.chunks {
			ch <- chunk
		}
	}()
	return ch, nil
}

func (m *mockStreamAI) ChatWithTools(_ context.Context, _ ai.ChatWithToolsRequest) (*ai.ChatWithToolsResponse, error) {
	return &ai.ChatWithToolsResponse{}, nil
}

func newTestRuntime(t *testing.T, aiProvider ai.AIProvider) *Runtime {
	t.Helper()
	memSvc := memory.NewService(nil, nil, nil, nil)
	emoSvc := emotion.NewService(nil, nil)
	briefSvc := brief.NewService(nil, config.GrowthConfig{})
	bondSvc := bond.NewService(nil)
	rt := NewRuntime(nil, aiProvider, memSvc, nil, nil, bondSvc, emoSvc, briefSvc, nil, config.GrowthConfig{}, nil, config.ToolsConfig{}, config.AIConfig{})
	rt.testOverridePet = &models.Pet{
		ID:              1,
		UserID:          1,
		Name:            "Mochi",
		Species:         "cat",
		BornAt:          time.Now().Add(-30 * 24 * time.Hour),
		IsAlive:         true,
		LifeStage:       "child",
		PersonalityJSON: []byte(`{"warmth":95,"empathy":95}`),
	}
	rt.testAgeInfo = lifecycle.AgeInfo{IsAlive: true, Stage: "child", AgeDays: 30}
	rt.testSkipPostProcess = true
	return rt
}

func TestRuntime_Perceive(t *testing.T) {
	rt := &Runtime{}
	res := rt.Perceive("我真的太难过了")
	if res.UserMood != "stressed" {
		t.Errorf("expected UserMood stressed, got %s", res.UserMood)
	}
	if res.Intensity != 0.8 {
		t.Errorf("expected intensity 0.8, got %v", res.Intensity)
	}
}

func TestRuntime_DecideStyle(t *testing.T) {
	rt := &Runtime{}

	healPet := &models.Pet{
		PersonalityJSON: []byte(`{"warmth":95,"empathy":95,"energy":40,"strictness":10}`),
	}
	perception := PerceptionResult{NeedsEmpathy: true, UserMood: "stressed", Intent: "vent"}
	bondProfile := models.BondProfile{}

	style1 := rt.DecideStyle(healPet, perception, bondProfile, "worried", false)
	if style1.SentenceLength != "medium" {
		t.Errorf("expected healPet sentence length medium, got %s", style1.SentenceLength)
	}
	if style1.EmojiRate <= 0 {
		t.Errorf("expected healPet emoji rate > 0, got %f", style1.EmojiRate)
	}

	tsunderePet := &models.Pet{
		PersonalityJSON: []byte(`{"warmth":70,"humor":90,"sarcasm":60,"confidence":90}`),
	}
	style2 := rt.DecideStyle(tsunderePet, PerceptionResult{}, bondProfile, "calm", false)
	if style2.Nickname != "家伙" {
		t.Errorf("expected tsundere nickname 家伙, got %s", style2.Nickname)
	}
}

func TestRuntime_DecideStyle_FocusWorkMode(t *testing.T) {
	rt := &Runtime{}
	pet := &models.Pet{
		PersonalityJSON: []byte(`{"warmth":90,"energy":80}`),
	}
	bondProfile := models.BondProfile{}

	style := rt.DecideStyle(pet, PerceptionResult{}, bondProfile, "calm", true)
	if style.SentenceLength != "short" {
		t.Errorf("expected focus mode sentence length short, got %s", style.SentenceLength)
	}
	if style.EmojiRate != 0.1 {
		t.Errorf("expected focus mode emoji rate 0.1, got %f", style.EmojiRate)
	}
	hasFocusModifier := false
	for _, tone := range style.ToneModifiers {
		if tone == "专注辅助/不打扰" {
			hasFocusModifier = true
		}
	}
	if !hasFocusModifier {
		t.Errorf("expected focus mode tone modifiers to contain 专注辅助/不打扰")
	}
}

func TestTransitionFSM(t *testing.T) {
	highEmpathy := 95
	lowEmpathy := 30

	state1, mood1 := TransitionFSM("calm", PerceptionResult{Intent: "joke"}, highEmpathy, "")
	if state1 != "happy" || mood1 != 85 {
		t.Errorf("calm + joke -> happy, got %s (%d)", state1, mood1)
	}

	state2, mood2 := TransitionFSM("happy", PerceptionResult{Intent: "joke"}, highEmpathy, "")
	if state2 != "excited" || mood2 != 100 {
		t.Errorf("happy + joke -> excited, got %s (%d)", state2, mood2)
	}

	state3, mood3 := TransitionFSM("sad", PerceptionResult{Intent: "joke"}, highEmpathy, "")
	if state3 != "happy" || mood3 != 85 {
		t.Errorf("sad + joke -> happy, got %s (%d)", state3, mood3)
	}

	state4, mood4 := TransitionFSM("calm", PerceptionResult{UserMood: "stressed", Intent: "vent"}, highEmpathy, "")
	if state4 != "worried" || mood4 != 45 {
		t.Errorf("calm + vent -> worried, got %s (%d)", state4, mood4)
	}

	state5, mood5 := TransitionFSM("happy", PerceptionResult{UserMood: "stressed", Intent: "vent"}, highEmpathy, "")
	if state5 != "worried" || mood5 != 45 {
		t.Errorf("happy + vent -> worried, got %s (%d)", state5, mood5)
	}

	state6, mood6 := TransitionFSM("worried", PerceptionResult{UserMood: "stressed", Intent: "vent"}, highEmpathy, "")
	if state6 != "sad" || mood6 != 20 {
		t.Errorf("worried + vent -> sad, got %s (%d)", state6, mood6)
	}

	state7, _ := TransitionFSM("calm", PerceptionResult{UserMood: "stressed", Intent: "vent"}, lowEmpathy, "")
	if state7 != "calm" {
		t.Errorf("low empathy pet should not transition on vent, got %s", state7)
	}

	state8, mood8 := TransitionFSM("excited", PerceptionResult{}, highEmpathy, "")
	if state8 != "happy" || mood8 != 85 {
		t.Errorf("excited decay -> happy, got %s (%d)", state8, mood8)
	}

	state9, mood9 := TransitionFSM("happy", PerceptionResult{}, highEmpathy, "")
	if state9 != "calm" || mood9 != 70 {
		t.Errorf("happy decay -> calm, got %s (%d)", state9, mood9)
	}

	// Phase 3: hold 期间 neutral 不 decay
	state10, _ := TransitionFSM("worried", PerceptionResult{UserMood: "neutral", Intent: "chat"}, highEmpathy, "worried")
	if state10 != "worried" {
		t.Errorf("worried hold + neutral -> worried, got %s", state10)
	}

	// Phase 4: sad 渐退 → worried
	state11, mood11 := TransitionFSM("sad", PerceptionResult{UserMood: "neutral", Intent: "chat"}, highEmpathy, "")
	if state11 != "worried" || mood11 != 45 {
		t.Errorf("sad decay -> worried, got %s (%d)", state11, mood11)
	}

	// E2E fix: cached stressed 无 NeedsEmpathy 不触发负向
	state12, _ := TransitionFSM("calm", PerceptionResult{UserMood: "stressed", Intent: "ask", NeedsEmpathy: false}, highEmpathy, "")
	if state12 != "calm" {
		t.Errorf("stressed without empathy should not transition, got %s", state12)
	}
}

func TestRuntime_Turn_triggersPostProcessAfterStream(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "抱抱你"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)
	rt.testPostProcessNotify = make(chan struct{}, 1)

	out, err := rt.Turn(context.Background(), TurnInput{
		UserID: 1, PetID: 1, Message: "我今天好难过", TriggerType: "user_chat",
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}
	for range out.ReplyStream {
	}

	select {
	case <-rt.testPostProcessNotify:
	case <-time.After(2 * time.Second):
		t.Fatal("expected postProcess after stream completion")
	}
}

func TestRuntime_UpdateEmotionFSM_nilDB(t *testing.T) {
	rt := &Runtime{}
	state, anim := rt.UpdateEmotionFSM(context.Background(), 1, PerceptionResult{Intent: "joke"})
	if state != "calm" || anim != "idle" {
		t.Fatalf("nil db should noop, got %s/%s", state, anim)
	}
}

func TestRuntime_FocusWorkMode(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "嗯"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)

	out, err := rt.Turn(context.Background(), TurnInput{
		UserID:      1,
		PetID:       1,
		Message:     "继续写代码",
		TriggerType: "user_chat",
		ActivityContext: map[string]interface{}{
			"active_app":                "Cursor.exe",
			"continuous_active_minutes": 20,
		},
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}
	for range out.ReplyStream {
	}
	if out.Trace.PersonalityDecision.Strategy != "short" {
		t.Errorf("expected short sentence length, got %s", out.Trace.PersonalityDecision.Strategy)
	}
	if !strings.Contains(out.Trace.PersonalityDecision.Notes, "专注辅助/不打扰") {
		t.Errorf("expected focus tone in notes, got %s", out.Trace.PersonalityDecision.Notes)
	}

	out2, err := rt.Turn(context.Background(), TurnInput{
		UserID:      1,
		PetID:       1,
		Message:     "你好",
		TriggerType: "user_chat",
		ActivityContext: map[string]interface{}{
			"active_app":                "Cursor.exe",
			"continuous_active_minutes": 10,
		},
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}
	for range out2.ReplyStream {
	}
	if out2.Trace.PersonalityDecision.Strategy == "short" &&
		strings.Contains(out2.Trace.PersonalityDecision.Notes, "专注辅助/不打扰") {
		t.Error("expected non-focus mode when only app matches without 15 min active")
	}
}

func TestRuntime_SystemProactive(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "该喝水啦！"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)

	msg := "[SYSTEM_TRIGGER: wellness_nudge] 照护类型: wellness_drink"
	out, err := rt.Turn(context.Background(), TurnInput{
		UserID: 1, PetID: 1, Message: msg, TriggerType: "system_proactive",
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}

	var reply string
	for chunk := range out.ReplyStream {
		if chunk.Content != "" {
			reply += chunk.Content
		}
	}
	if reply != "该喝水啦！" {
		t.Errorf("expected proactive reply '该喝水啦！', got %s", reply)
	}

	found := false
	for _, m := range mockAI.lastMessages {
		if strings.Contains(m.Content, "【系统指令 - 主动关怀】") &&
			strings.Contains(m.Content, msg) &&
			strings.Contains(m.Content, "20字") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected assembled prompt to contain proactive instruction block")
	}
}

func TestRuntime_Turn_SystemProactive(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "该喝水啦！"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)
	rt.testSkipPostProcess = true

	out, err := rt.Turn(context.Background(), TurnInput{
		UserID: 1, PetID: 1, Message: "提醒喝水", TriggerType: "system_proactive",
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}

	var reply string
	for chunk := range out.ReplyStream {
		if chunk.Content != "" {
			reply += chunk.Content
		}
	}

	if reply != "该喝水啦！" {
		t.Errorf("expected proactive reply '该喝水啦！', got %s", reply)
	}
}

func TestRuntime_Turn_PipelinePerception(t *testing.T) {
	mockAI := &mockStreamAI{
		chunks: []ai.ChatChunk{{Content: "抱抱你", Speech: "抱抱你"}, {Done: true}},
	}
	rt := newTestRuntime(t, mockAI)

	pipeline := &emotion.PerceptionState{
		Hint: emotion.Hint{
			UserMood:     "stressed",
			Intent:       "vent",
			NeedsEmpathy: true,
			Temperature:  0.75,
		},
		Source: "pipeline_v3c",
	}

	out, err := rt.Turn(context.Background(), TurnInput{
		UserID: 1, PetID: 1, Message: "年龄又大了一岁", TriggerType: "user_voice",
		PipelinePerception: pipeline,
	})
	if err != nil {
		t.Fatalf("Turn failed: %v", err)
	}
	for range out.ReplyStream {
	}

	if out.Trace.Perception.UserMood != "stressed" {
		t.Errorf("trace mood=%s want stressed", out.Trace.Perception.UserMood)
	}
	if out.Trace.Perception.Intent != "vent" {
		t.Errorf("trace intent=%s want vent", out.Trace.Perception.Intent)
	}
	if !out.Trace.Perception.NeedsEmpathy {
		t.Error("expected needs_empathy true in trace")
	}
}
