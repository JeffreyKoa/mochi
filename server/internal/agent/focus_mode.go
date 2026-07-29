package agent

import "strings"

const focusActiveMinutesThreshold = 15

func isFocusApplication(app string) bool {
	app = strings.ToLower(app)
	focusList := []string{
		"cursor", "cursor.exe",
		"vscode", "code.exe", "visual studio code",
		"goland", "xcode", "intellij", "idea64",
		"terminal", "windowsterminal", "wt.exe", "cmd", "cmd.exe", "powershell",
		"obsidian", "word", "winword", "excel", "wpf", "notion",
		"figma", "photoshop", "illustrator",
	}
	for _, f := range focusList {
		if strings.Contains(app, f) {
			return true
		}
	}
	return false
}

// IsFocusWorkMode reports whether the owner is in focus work mode (focus app AND active >= 15 min).
func IsFocusWorkMode(activeApp string, continuousActiveMinutes int) bool {
	if activeApp == "" || continuousActiveMinutes < focusActiveMinutesThreshold {
		return false
	}
	return isFocusApplication(activeApp)
}

// FocusModeFromActivityContext evaluates focus mode from a Turn activity context map.
func FocusModeFromActivityContext(ctx map[string]interface{}) bool {
	if ctx == nil {
		return false
	}
	activeApp := stringField(ctx, "active_app")
	mins := intField(ctx, "continuous_active_minutes")
	return IsFocusWorkMode(activeApp, mins)
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func intField(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
