package ai

import (
	"context"
	"errors"
	"testing"
)

type mockProvider struct {
	name      string
	failChat  bool
	chatReply string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.failChat {
		return nil, errors.New("mock chat error")
	}
	return &ChatResponse{Content: m.chatReply}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error) {
	if m.failChat {
		return nil, errors.New("mock stream error")
	}
	ch := make(chan ChatChunk, 1)
	ch <- ChatChunk{Content: m.chatReply, Done: true}
	close(ch)
	return ch, nil
}

func (m *mockProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (*ChatWithToolsResponse, error) {
	if m.failChat {
		return nil, errors.New("mock tools error")
	}
	return &ChatWithToolsResponse{Content: m.chatReply}, nil
}

func TestRouter_Failover(t *testing.T) {
	router := NewRouter()

	if name := router.Name(); name != "ai-router" {
		t.Errorf("expected name ai-router, got %s", name)
	}

	p1 := &mockProvider{name: "p1", failChat: true}
	p2 := &mockProvider{name: "p2", failChat: false, chatReply: "hello from p2"}

	router.Register("p1", p1, false)
	router.Register("p2", p2, true)

	resp, err := router.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("expected failover to succeed, got error: %v", err)
	}
	if resp.Content != "hello from p2" {
		t.Errorf("expected content 'hello from p2', got '%s'", resp.Content)
	}
}
