package vision

import "testing"

func TestParseVLResponse_Face(t *testing.T) {
	raw := `{"user_expression":"tired","confidence":0.85,"note":"主人眼皮有些下垂，像没休息好"}`
	h := parseVLResponse(FocusOwnerFace, raw)
	if h.UserExpression != "tired" {
		t.Fatalf("expr=%s", h.UserExpression)
	}
	if h.ExpressionConfidence < 0.8 {
		t.Fatalf("conf=%f", h.ExpressionConfidence)
	}
}

func TestParseVLResponse_Object(t *testing.T) {
	raw := `{"object_summary":"红色马克杯","note":"一个带把手的红色陶瓷杯"}`
	h := parseVLResponse(FocusObject, raw)
	if h.ObjectSummary != "红色马克杯" {
		t.Fatalf("object=%s", h.ObjectSummary)
	}
	if !h.IsUsable() {
		t.Fatal("object hint should be usable")
	}
}

func TestParseVLResponse_Scene(t *testing.T) {
	raw := `{"scene_summary":"整洁的书桌，自然光从左侧照入","note":"像是在居家办公"}`
	h := parseVLResponse(FocusScene, raw)
	if h.SceneSummary == "" {
		t.Fatal("empty scene")
	}
	if !h.IsUsable() {
		t.Fatal("scene hint should be usable")
	}
}

func TestHint_IsUsable(t *testing.T) {
	if EmptyHint().IsUsable() {
		t.Fatal("empty should not be usable")
	}
	h := Hint{Focus: FocusOwnerFace, Note: "test", ExpressionConfidence: 0.7}
	if !h.IsUsable() {
		t.Fatal("should be usable")
	}
}
