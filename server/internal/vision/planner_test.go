package vision

import "testing"

func TestPlanContextual_HeuristicObjectWhenClassifyNone(t *testing.T) {
	plan := PlanContextual("你看我手里拿的是啥东西？", Hint{}, "none", false)
	if !plan.NeedSecondVL || plan.Focus != FocusObject {
		t.Fatalf("expected heuristic object refine, got %+v", plan)
	}
}

func TestInferVisualTaskFromText(t *testing.T) {
	if got := InferVisualTaskFromText("你看我手里拿的是啥"); got != "object" {
		t.Fatalf("expected object, got %q", got)
	}
	if got := InferVisualTaskFromText("今天好累"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestPlanContextual_Object(t *testing.T) {
	plan := PlanContextual("看我手里拿的这个是什么东西", Hint{}, "object", false)
	if !plan.NeedSecondVL || plan.Focus != FocusObject {
		t.Fatalf("expected object contextual refine, got %+v", plan)
	}
	if plan.DynamicPrompt == "" {
		t.Fatal("expected dynamic prompt")
	}
}

func TestPlanContextual_FaceConsistency(t *testing.T) {
	face := Hint{Focus: FocusOwnerFace, UserExpression: "tired", ExpressionConfidence: 0.8, Note: "疲惫"}
	plan := PlanContextual("是是，我没事", face, "face_consistency", true)
	if !plan.NeedSecondVL || plan.Focus != FocusOwnerFace {
		t.Fatalf("expected face consistency refine, got %+v", plan)
	}
}

func TestPlanContextual_SkipChat(t *testing.T) {
	plan := PlanContextual("今天天气不错", Hint{Focus: FocusOwnerFace, UserExpression: "happy"}, "none", false)
	if plan.NeedSecondVL {
		t.Fatalf("chat should skip second VL, got %+v", plan)
	}
}

func TestPlanRefine_Object(t *testing.T) {
	plan := PlanRefine("你看这是什么", []string{"这是什么"}, []string{"你看外面"})
	if !plan.NeedSecondVL || plan.Focus != FocusObject {
		t.Fatalf("expected object refine, got %+v", plan)
	}
}

func TestPlanRefine_OwnerFaceDefault(t *testing.T) {
	plan := PlanRefine("最近好累啊", []string{"这是什么"}, []string{"你看外面"})
	if plan.NeedSecondVL {
		t.Fatalf("vent chat should not second VL, got %+v", plan)
	}
}
