package agent

import "testing"

func TestIsFocusWorkMode_ANDLogic(t *testing.T) {
	tests := []struct {
		name  string
		app   string
		mins  int
		want  bool
	}{
		{"both match", "Cursor.exe", 20, true},
		{"app only", "Cursor.exe", 10, false},
		{"minutes only", "Notepad.exe", 20, false},
		{"neither", "Notepad.exe", 5, false},
		{"empty app", "", 20, false},
		{"vscode exe", "Code.exe", 15, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFocusWorkMode(tt.app, tt.mins); got != tt.want {
				t.Errorf("IsFocusWorkMode(%q, %d) = %v, want %v", tt.app, tt.mins, got, tt.want)
			}
		})
	}
}

func TestFocusModeFromActivityContext(t *testing.T) {
	ctx := map[string]interface{}{
		"active_app":                "Cursor.exe",
		"continuous_active_minutes": 20,
	}
	if !FocusModeFromActivityContext(ctx) {
		t.Fatal("expected focus mode from activity context")
	}
	ctx["continuous_active_minutes"] = 10
	if FocusModeFromActivityContext(ctx) {
		t.Fatal("expected non-focus when minutes below threshold")
	}
}
