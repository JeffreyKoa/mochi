package vision

import "testing"

func TestIsVLTemplateGarbage(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"Mochi", true},
		{`"Mochi"`, true},
		{`"Mochi 第一人称中文"`, true},
		{`{"object_summary":"手机","note":"我看见你手里是部手机"}`, false},
		{"我看见你手里是部黑色手机", false},
	}
	for _, c := range cases {
		if got := isVLTemplateGarbage(c.raw); got != c.want {
			t.Fatalf("isVLTemplateGarbage(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestParseVLResponse_RejectsTemplateGarbage(t *testing.T) {
	h := parseVLResponse(FocusOwnerFace, "Mochi")
	if !h.Skipped || h.SkipReason != "vl_template_garbage" {
		t.Fatalf("expected skipped template garbage, got %+v", h)
	}
	h = parseVLResponse(FocusObject, `"Mochi 第一人称中文"`)
	if !h.Skipped {
		t.Fatalf("expected object template garbage skipped, got %+v", h)
	}
}

func TestWantsHardFocusVision(t *testing.T) {
	keys := []string{"手里", "这是什么"}
	if !WantsHardFocusVision("你看我手里拿的是啥", keys, nil) {
		t.Fatal("expected hard focus for object question")
	}
	if WantsHardFocusVision("今天好累", keys, nil) {
		t.Fatal("vent chat should not want hard focus")
	}
}
