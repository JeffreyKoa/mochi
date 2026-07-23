package prompt

import (
	"github.com/mochi-ai/server/internal/models"
)

func formatMemories(memories []models.Memory) string {
	return formatCompanionMemoriesBudget(memories, defaultMemoryPromptBudget)
}

func describeMood(mood uint8) string {
	switch {
	case mood >= 80:
		return "非常开心"
	case mood >= 60:
		return "心情不错"
	case mood >= 40:
		return "一般般"
	case mood >= 20:
		return "有点低落"
	default:
		return "很难过"
	}
}
