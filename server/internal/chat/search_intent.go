package chat

import "github.com/mochi-ai/server/internal/agent"

// NeedsWebSearch returns true when the user message likely needs up-to-date web information.
func NeedsWebSearch(msg string) bool {
	return agent.NeedsWebSearch(msg)
}
