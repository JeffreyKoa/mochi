package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/pkg/ai"
)

type Orchestrator struct {
	svc *Service
	ai  *ai.Provider
	cfg config.ToolsConfig
}

func NewOrchestrator(svc *Service, aiProvider *ai.Provider, cfg config.ToolsConfig) *Orchestrator {
	return &Orchestrator{svc: svc, ai: aiProvider, cfg: cfg}
}

type HandleResult struct {
	Ran      bool
	ToolNote string
}

func (o *Orchestrator) Handle(ctx context.Context, pet *models.Pet, userID uint64, userMsg string, hint emotion.Hint, bond models.BondProfile) HandleResult {
	if o.svc == nil || !o.svc.Enabled() {
		return HandleResult{}
	}
	if hint.NeedsEmpathy || hint.Intent == "vent" {
		if !hasExplicitToolRequest(userMsg) {
			return HandleResult{}
		}
	}

	tool, title, fireAt, id := o.route(userMsg, hint)
	if tool == "" && o.cfg.RouterLLMEnabled && o.ai != nil {
		tool, title, fireAt, id = o.routeLLM(ctx, userMsg)
	}
	if tool == "" {
		return HandleResult{}
	}

	if bond.TrustLevel < uint8(o.cfg.MinTrustForAutoCreate) && hint.Intent != "plan" && !hasExplicitToolRequest(userMsg) {
		return HandleResult{}
	}

	now := time.Now()
	var note string
	var err error

	switch tool {
	case "reminder_create":
		if title == "" {
			title = ExtractReminderTitle(userMsg)
		}
		if !fireAt.IsZero() {
			_, err = o.svc.CreateReminder(ctx, pet.ID, userID, title, fireAt, userMsg)
			if err == nil {
				note = fmt.Sprintf("【刚帮主人办了】已创建提醒「%s」，时间 %s。请用 1~2 句口语确认，不要说「已为您创建」。", title, FormatFireAt(fireAt))
			}
		} else {
			note = "【待确认】主人想设提醒但时间不清楚，请温柔问「几点提醒你呀？」不要假装已创建。"
		}
	case "reminder_list":
		list, e := o.svc.ListReminders(ctx, pet.ID, "pending", 10)
		err = e
		if err == nil {
			if len(list) == 0 {
				note = "【刚帮主人查了】目前没有待提醒事项。口语告诉主人即可。"
			} else {
				var lines []string
				for _, r := range list {
					lines = append(lines, fmt.Sprintf("- %s（%s）", r.Title, FormatFireAt(r.FireAt)))
				}
				note = "【刚帮主人查了】待提醒：\n" + strings.Join(lines, "\n") + "\n请口语概括，不要列表腔。"
			}
		}
	case "reminder_cancel":
		if id > 0 {
			err = o.svc.CancelReminder(ctx, pet.ID, id)
		} else {
			_, err = o.svc.CancelReminderByTitle(ctx, pet.ID, title)
		}
		if err == nil {
			note = "【刚帮主人办了】已取消相关提醒。口语确认即可。"
		}
	case "todo_add":
		if title == "" {
			title = ExtractTodoTitle(userMsg)
		}
		var due *time.Time
		if t, ok := ParseFireAt(userMsg, now); ok {
			due = &t
		}
		_, err = o.svc.AddTodo(ctx, pet.ID, userID, title, due)
		if err == nil {
			note = fmt.Sprintf("【刚帮主人办了】已记下待办「%s」。口语确认，像伙伴答应帮忙。", title)
		}
	case "todo_list":
		list, e := o.svc.ListTodos(ctx, pet.ID, false, 15)
		err = e
		if err == nil {
			if len(list) == 0 {
				note = "【刚帮主人查了】待办清单是空的~"
			} else {
				var lines []string
				for _, t := range list {
					lines = append(lines, "- "+t.Title)
				}
				note = "【刚帮主人查了】未完成待办：\n" + strings.Join(lines, "\n")
			}
		}
	case "todo_complete":
		if id > 0 {
			err = o.svc.CompleteTodo(ctx, pet.ID, id)
		} else {
			_, err = o.svc.CompleteTodoByTitle(ctx, pet.ID, title)
		}
		if err == nil {
			note = "【刚帮主人办了】待办已勾选完成~"
		}
	}

	if err != nil || note == "" {
		return HandleResult{}
	}
	return HandleResult{Ran: true, ToolNote: note}
}

func hasExplicitToolRequest(msg string) bool {
	keywords := []string{
		"提醒", "记得", "别忘了", "闹钟", "待办", "todo",
		"记一下", "记下来", "记下", "帮我记", "帮我把",
		"有哪些提醒", "有哪些待办", "取消提醒", "完成了", "做完了",
	}
	lower := strings.ToLower(msg)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

func (o *Orchestrator) route(msg string, hint emotion.Hint) (tool, title string, fireAt time.Time, id uint64) {
	lower := strings.ToLower(msg)

	if strings.Contains(msg, "取消") && strings.Contains(msg, "提醒") {
		return "reminder_cancel", ExtractReminderTitle(msg), time.Time{}, 0
	}
	if strings.Contains(msg, "哪些提醒") || strings.Contains(msg, "什么提醒") {
		return "reminder_list", "", time.Time{}, 0
	}
	if strings.Contains(msg, "哪些待办") || strings.Contains(msg, "什么待办") {
		return "todo_list", "", time.Time{}, 0
	}
	if (strings.Contains(msg, "完成") || strings.Contains(msg, "做完") || strings.Contains(msg, "勾选")) &&
		(strings.Contains(msg, "待办") || strings.Contains(lower, "todo")) {
		return "todo_complete", ExtractTodoTitle(msg), time.Time{}, 0
	}

	if looksLikeTimedReminder(msg, hint.Intent) {
		title = ExtractReminderTitle(msg)
		fireAt, _ = ParseFireAt(msg, time.Now())
		return "reminder_create", title, fireAt, 0
	}

	if strings.Contains(msg, "提醒") || strings.Contains(msg, "记得") || strings.Contains(msg, "别忘了") || strings.Contains(msg, "闹钟") {
		title = ExtractReminderTitle(msg)
		fireAt, _ = ParseFireAt(msg, time.Now())
		return "reminder_create", title, fireAt, 0
	}

	if strings.Contains(msg, "待办") || strings.Contains(lower, "todo") ||
		strings.Contains(msg, "记一下") || strings.Contains(msg, "记下来") ||
		strings.Contains(msg, "记下") || strings.Contains(msg, "帮我记") ||
		strings.Contains(msg, "帮我把") {
		return "todo_add", ExtractTodoTitle(msg), time.Time{}, 0
	}

	if hint.Intent == "plan" && (strings.Contains(msg, "记") || strings.Contains(msg, "安排")) {
		fireAt, ok := ParseFireAt(msg, time.Now())
		if ok {
			return "reminder_create", ExtractReminderTitle(msg), fireAt, 0
		}
		return "todo_add", ExtractTodoTitle(msg), time.Time{}, 0
	}

	return "", "", time.Time{}, 0
}

func (o *Orchestrator) routeLLM(ctx context.Context, userMsg string) (tool, title string, fireAt time.Time, id uint64) {
	prompt := fmt.Sprintf(`判断用户是否要操作提醒/待办。时区 UTC+8，现在 %s。
返回 JSON：{"tool":"reminder_create|reminder_list|reminder_cancel|todo_add|todo_list|todo_complete|null","title":"","fire_at":"ISO8601或空","id":0}
用户：%s`, time.Now().In(loc).Format(time.RFC3339), userMsg)

	resp, err := o.ai.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: "user", Content: prompt}},
		Temperature: 0.1,
		MaxTokens:   200,
	})
	if err != nil {
		return "", "", time.Time{}, 0
	}
	var out struct {
		Tool   string `json:"tool"`
		Title  string `json:"title"`
		FireAt string `json:"fire_at"`
		ID     uint64 `json:"id"`
	}
	raw := strings.TrimSpace(resp.Content)
	if i := strings.Index(raw, "{"); i >= 0 {
		raw = raw[i:]
	}
	if j := strings.LastIndex(raw, "}"); j >= 0 {
		raw = raw[:j+1]
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.Tool == "" || out.Tool == "null" {
		return "", "", time.Time{}, 0
	}
	if out.FireAt != "" {
		if t, err := time.Parse(time.RFC3339, out.FireAt); err == nil {
			fireAt = t
		}
	}
	return out.Tool, out.Title, fireAt, out.ID
}
