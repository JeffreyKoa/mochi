package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mochi-ai/server/pkg/ai"
)

// HeuristicResult is a synthetic tool execution outcome for confirmation prompting.
type HeuristicResult struct {
	ToolName string
	Content  string
}

// TryHeuristicCreate attempts rule-based reminder/todo creation when the LLM skipped tools.
func (e *Executor) TryHeuristicCreate(ctx context.Context, exec ExecContext) (*HeuristicResult, error) {
	if e.svc == nil || !e.Enabled() {
		return nil, nil
	}
	if e.requiresTrust("reminder_create") && exec.Bond.TrustLevel < uint8(e.cfg.MinTrustForAutoCreate) {
		return nil, nil
	}

	msg := exec.UserMsg
	if looksLikeReminder(msg) {
		fireAt, ok := ParseScheduledTime(msg)
		if !ok {
			return nil, nil
		}
		title := inferReminderTitle(msg)
		r, err := e.svc.CreateReminder(ctx, exec.PetID, exec.UserID, title, fireAt, msg)
		if err != nil {
			return nil, err
		}
		log.Printf("[tools] heuristic reminder_create id=%d pet=%d fire_at=%s title=%q", r.ID, exec.PetID, r.FireAt.In(loc).Format(time.RFC3339), r.Title)
		payload, _ := json.Marshal(map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"id":      r.ID,
				"title":   r.Title,
				"fire_at": r.FireAt.Format(time.RFC3339),
			},
		})
		return &HeuristicResult{ToolName: "reminder_create", Content: string(payload)}, nil
	}

	if looksLikeTodo(msg) && !looksLikeReminder(msg) {
		title := ExtractTodoTitle(msg)
		if title == "" {
			return nil, nil
		}
		t, err := e.svc.AddTodo(ctx, exec.PetID, exec.UserID, title, nil)
		if err != nil {
			return nil, err
		}
		log.Printf("[tools] heuristic todo_add id=%d pet=%d title=%q", t.ID, exec.PetID, t.Title)
		payload, _ := json.Marshal(map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{"id": t.ID, "title": t.Title},
		})
		return &HeuristicResult{ToolName: "todo_add", Content: string(payload)}, nil
	}

	return nil, nil
}

// AppendHeuristicToolTurn builds messages for the confirmation LLM pass after heuristic create.
func AppendHeuristicToolTurn(msgs []ai.Message, hr *HeuristicResult) []ai.Message {
	out := append([]ai.Message{}, msgs...)
	out = append(out, ai.Message{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ai.ToolCall{{
			ID:   "heuristic-1",
			Type: "function",
			Function: ai.FunctionCall{
				Name:      hr.ToolName,
				Arguments: "{}",
			},
		}},
	})
	out = append(out, ai.Message{
		Role:       "tool",
		ToolCallID: "heuristic-1",
		Name:       hr.ToolName,
		Content:    hr.Content,
	})
	out = append(out, ai.Message{
		Role:    "user",
		Content: fmt.Sprintf("【系统】已通过 %s 完成办事。请用1-2句口语向主人确认结果（含时间/事项），禁止用括号描述动作。", hr.ToolName),
	})
	return out
}
