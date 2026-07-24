package tools

import (
	"encoding/json"

	"github.com/mochi-ai/server/pkg/ai"
)

// Registry returns OpenAI-format tool definitions for reminder/todo operations.
func Registry() []ai.ToolDefinition {
	return []ai.ToolDefinition{
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "reminder_create",
				Description: "当用户明确要求在某个时间提醒他做某事时调用。用户时区 UTC+8。不要在不明确时间或用户只是闲聊时调用。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {"type": "string", "description": "提醒事项，简短"},
						"fire_at": {"type": "string", "description": "ISO8601 时间，含时区偏移，例如 2026-07-24T09:00:00+08:00"}
					},
					"required": ["title", "fire_at"]
				}`),
			},
		},
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "reminder_list",
				Description: "用户询问有哪些待提醒、提醒列表时调用",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"limit": {"type": "integer", "description": "最多返回条数，默认10"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "reminder_cancel",
				Description: "用户要求取消某个提醒时调用",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"id": {"type": "integer", "description": "提醒 ID"},
						"title_match": {"type": "string", "description": "按标题模糊匹配取消"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "todo_add",
				Description: "当用户要求记下待办、买东西、记一件事（无具体提醒时间）时调用",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {"type": "string", "description": "待办内容"},
						"due_at": {"type": "string", "description": "可选 ISO8601 截止时间"}
					},
					"required": ["title"]
				}`),
			},
		},
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "todo_list",
				Description: "用户询问待办清单、有哪些要做的事时调用",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"limit": {"type": "integer", "description": "最多返回条数，默认15"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: ai.FunctionSchema{
				Name:        "todo_complete",
				Description: "用户表示某待办已完成、要勾选完成时调用",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"id": {"type": "integer", "description": "待办 ID"},
						"title_match": {"type": "string", "description": "按标题模糊匹配完成"}
					}
				}`),
			},
		},
	}
}
