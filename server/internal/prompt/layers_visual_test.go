package prompt

import "testing"

func TestFormatVisualIdentityBlock(t *testing.T) {
	if got := formatVisualIdentityBlock("owner"); got == "" {
		t.Fatal("expected owner block")
	}
	if got := formatVisualIdentityBlock("unknown"); got == "" {
		t.Fatal("expected unknown block")
	}
	if got := formatVisualIdentityBlock(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
