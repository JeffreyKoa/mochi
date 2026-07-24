package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

type ExecContext struct {
	PetID   uint64
	UserID  uint64
	UserMsg string
	Bond    models.BondProfile
}

type Executor struct {
	svc *Service
	cfg config.ToolsConfig
}

func NewExecutor(svc *Service, cfg config.ToolsConfig) *Executor {
	return &Executor{svc: svc, cfg: cfg}
}

func (e *Executor) Enabled() bool {
	return e.svc != nil && e.svc.Enabled() && (e.cfg.Mode == "" || e.cfg.Mode == "tool_calling")
}

func (e *Executor) Run(ctx context.Context, exec ExecContext, call ai.ToolCall) string {
	if e.svc == nil {
		return failJSON("tools unavailable")
	}
	name := call.Function.Name
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return failJSON("invalid arguments: " + err.Error())
	}

	if e.requiresTrust(name) && exec.Bond.TrustLevel < uint8(e.cfg.MinTrustForAutoCreate) {
		return failJSON("trust too low")
	}

	var err error
	var data interface{}

	switch name {
	case "reminder_create":
		data, err = e.reminderCreate(ctx, exec, args)
	case "reminder_list":
		data, err = e.reminderList(ctx, exec, args)
	case "reminder_cancel":
		data, err = e.reminderCancel(ctx, exec, args)
	case "todo_add":
		data, err = e.todoAdd(ctx, exec, args)
	case "todo_list":
		data, err = e.todoList(ctx, exec, args)
	case "todo_complete":
		data, err = e.todoComplete(ctx, exec, args)
	default:
		return failJSON("unknown tool: " + name)
	}

	if err != nil {
		return failJSON(err.Error())
	}
	b, _ := json.Marshal(map[string]interface{}{"ok": true, "data": data})
	return string(b)
}

func (e *Executor) requiresTrust(name string) bool {
	switch name {
	case "reminder_create", "todo_add", "reminder_cancel", "todo_complete":
		return true
	default:
		return false
	}
}

func (e *Executor) reminderCreate(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	title := strArg(args, "title")
	fireRaw := strArg(args, "fire_at")
	fireAt, err := parseTimeISO(fireRaw, exec.UserMsg)
	if err != nil {
		return nil, fmt.Errorf("invalid fire_at: %w", err)
	}
	if title == "" {
		return nil, fmt.Errorf("title required")
	}
	r, err := e.svc.CreateReminder(ctx, exec.PetID, exec.UserID, title, fireAt, exec.UserMsg)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":      r.ID,
		"title":   r.Title,
		"fire_at": r.FireAt.Format(time.RFC3339),
	}, nil
}

func (e *Executor) reminderList(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	limit := intArg(args, "limit", 10)
	list, err := e.svc.ListReminders(ctx, exec.PetID, "pending", limit)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (e *Executor) reminderCancel(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	if id := uintArg(args, "id"); id > 0 {
		if err := e.svc.CancelReminder(ctx, exec.PetID, id); err != nil {
			return nil, err
		}
		return map[string]interface{}{"cancelled": 1}, nil
	}
	match := strArg(args, "title_match")
	if match == "" {
		return nil, fmt.Errorf("id or title_match required")
	}
	n, err := e.svc.CancelReminderByTitle(ctx, exec.PetID, match)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return map[string]interface{}{"cancelled": n}, nil
}

func (e *Executor) todoAdd(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	title := strArg(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title required")
	}
	var due *time.Time
	if dueRaw := strArg(args, "due_at"); dueRaw != "" {
		t, err := parseTimeISO(dueRaw, exec.UserMsg)
		if err == nil {
			due = &t
		}
	}
	t, err := e.svc.AddTodo(ctx, exec.PetID, exec.UserID, title, due)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": t.ID, "title": t.Title}, nil
}

func (e *Executor) todoList(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	limit := intArg(args, "limit", 15)
	list, err := e.svc.ListTodos(ctx, exec.PetID, false, limit)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (e *Executor) todoComplete(ctx context.Context, exec ExecContext, args map[string]interface{}) (interface{}, error) {
	if id := uintArg(args, "id"); id > 0 {
		if err := e.svc.CompleteTodo(ctx, exec.PetID, id); err != nil {
			return nil, err
		}
		return map[string]interface{}{"completed": 1}, nil
	}
	match := strArg(args, "title_match")
	if match == "" {
		return nil, fmt.Errorf("id or title_match required")
	}
	n, err := e.svc.CompleteTodoByTitle(ctx, exec.PetID, match)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return map[string]interface{}{"completed": n}, nil
}

func parseTimeISO(raw, fallback string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.In(loc), nil
		}
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, loc); err == nil {
			return t, nil
		}
	}
	if t, ok := ParseFireAt(fallback, time.Now()); ok {
		return t, nil
	}
	if raw != "" {
		if t, ok := ParseFireAt(raw, time.Now()); ok {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time")
}

func strArg(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func intArg(args map[string]interface{}, key string, def int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return def
	}
}

func uintArg(args map[string]interface{}, key string) uint64 {
	return uint64(intArg(args, key, 0))
}

func failJSON(msg string) string {
	b, _ := json.Marshal(map[string]interface{}{"ok": false, "error": msg})
	return string(b)
}
