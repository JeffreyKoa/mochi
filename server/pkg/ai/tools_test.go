package ai

import (
	"encoding/json"
	"testing"
)

func TestParseToolCallsResponse(t *testing.T) {
	raw := `{
		"choices": [{
			"message": {
				"content": "",
				"tool_calls": [{
					"id": "call_1",
					"type": "function",
					"function": {
						"name": "todo_add",
						"arguments": "{\"title\":\"买牛奶\"}"
					}
				}]
			}
		}]
	}`
	var result struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices[0].Message.ToolCalls) != 1 {
		t.Fatal("expected tool call")
	}
	if result.Choices[0].Message.ToolCalls[0].Function.Name != "todo_add" {
		t.Fatalf("name=%s", result.Choices[0].Message.ToolCalls[0].Function.Name)
	}
}
