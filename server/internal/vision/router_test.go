package vision

import "testing"

func TestRouteFocus_OwnerFaceOnVoiceTurn(t *testing.T) {
	f := RouteFocus(RouteInput{
		UserText:      "今天好累",
		Intent:        "chat",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
	})
	if f != FocusOwnerFace {
		t.Fatalf("expected owner_face, got %s", f)
	}
}

func TestRouteFocus_ObjectTrigger(t *testing.T) {
	f := RouteFocus(RouteInput{
		UserText:      "Mochi 这是什么",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
		ObjectKeys:    []string{"这是什么", "看看这个"},
	})
	if f != FocusObject {
		t.Fatalf("expected object, got %s", f)
	}
}

func TestRouteFocus_SceneTrigger(t *testing.T) {
	f := RouteFocus(RouteInput{
		UserText:      "你看外面天气怎么样",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
		SceneKeys:     []string{"你看外面", "看看外面"},
	})
	if f != FocusScene {
		t.Fatalf("expected scene, got %s", f)
	}
}

func TestRouteFocus_ObjectBeatsScene(t *testing.T) {
	f := RouteFocus(RouteInput{
		UserText:      "你看外面这是什么",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
		ObjectKeys:    []string{"这是什么"},
		SceneKeys:     []string{"你看外面"},
	})
	if f != FocusObject {
		t.Fatalf("object should win, got %s", f)
	}
}

func TestRouteFocus_SkipWhenDisabled(t *testing.T) {
	f := RouteFocus(RouteInput{VisionEnabled: false, HasFrame: true, IsVoiceTurn: true})
	if f != FocusSkip {
		t.Fatalf("expected skip, got %s", f)
	}
}

func TestRouteFocus_SkipWeatherTopic(t *testing.T) {
	topics := []string{"weather"}
	f := RouteFocus(RouteInput{
		UserText:      "深圳天气怎么样",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
		SkipTopics:    topics,
		DeicticExempt: true,
	})
	if f != FocusSkip {
		t.Fatalf("expected skip for weather, got %s", f)
	}
}

func TestRouteFocus_SceneNotMatchedOnChat(t *testing.T) {
	f := RouteFocus(RouteInput{
		UserText:      "今天天气不错",
		IsVoiceTurn:   true,
		VisionEnabled: true,
		HasFrame:      true,
		SceneKeys:     []string{"你看外面"},
	})
	if f != FocusOwnerFace {
		t.Fatalf("expected owner_face, got %s", f)
	}
}
