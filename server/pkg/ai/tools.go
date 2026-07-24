package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ToolCall is an OpenAI-compatible function call from the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition describes a callable function for the model.
type ToolDefinition struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ChatWithToolsRequest struct {
	Messages    []Message         `json:"messages"`
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition  `json:"tools"`
	ToolChoice  string            `json:"tool_choice,omitempty"`
}

type ChatWithToolsResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type chatMessagePayload struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

func messageToPayload(m Message) chatMessagePayload {
	p := chatMessagePayload{Role: m.Role, Content: m.Content}
	if len(m.ToolCalls) > 0 {
		p.ToolCalls = m.ToolCalls
	}
	if m.ToolCallID != "" {
		p.ToolCallID = m.ToolCallID
	}
	if m.Name != "" {
		p.Name = m.Name
	}
	return p
}

// ChatWithTools calls the chat completions API with tools (non-streaming).
func (p *Provider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (*ChatWithToolsResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	if req.ToolChoice == "" {
		req.ToolChoice = "auto"
	}

	msgs := make([]chatMessagePayload, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = messageToPayload(m)
	}

	body, err := json.Marshal(map[string]interface{}{
		"model":       req.Model,
		"messages":    msgs,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"tools":       req.Tools,
		"tool_choice": req.ToolChoice,
		"stream":      false,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty AI response")
	}
	msg := result.Choices[0].Message
	return &ChatWithToolsResponse{
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
	}, nil
}
