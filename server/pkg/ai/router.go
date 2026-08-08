package ai

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/mochi-ai/server/pkg/modelmeta"
)

// Router 多模型路由器：支持故障自动转移 (Failover) 与优先级配置
type Router struct {
	mu        sync.RWMutex
	providers map[string]AIProvider
	priority  []string // 主优先级提供者列表
	fallback  []string // 备用降级提供者列表
}

func NewRouter() *Router {
	return &Router{
		providers: make(map[string]AIProvider),
		priority:  make([]string, 0),
		fallback:  make([]string, 0),
	}
}

func (r *Router) Name() string {
	return "ai-router"
}

// Register 注册一个 AIProvider，可指定是否作为 fallback 备用提供者
func (r *Router) Register(name string, provider AIProvider, isFallback bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[name] = provider
	if isFallback {
		if !containsString(r.fallback, name) {
			r.fallback = append(r.fallback, name)
		}
	} else {
		if !containsString(r.priority, name) {
			r.priority = append(r.priority, name)
		}
	}
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// resolveModelVendor 从请求与 Provider 解析实际调用的模型与厂商。
func resolveModelVendor(reqModel string, provider AIProvider) (model, vendor string) {
	model = reqModel
	vendor = "unknown"
	if mi, ok := provider.(interface {
		Vendor() string
		DefaultModel() string
	}); ok {
		if model == "" {
			model = mi.DefaultModel()
		}
		vendor = mi.Vendor()
	}
	if model == "" {
		model = provider.Name()
	}
	return model, vendor
}

func logProviderAttempt(subsystem, route, mode, reqModel string, provider AIProvider) {
	model, vendor := resolveModelVendor(reqModel, provider)
	modelmeta.LogCall(subsystem, vendor, model, "route="+route, "mode="+mode)
}

// Chat 依序尝试主模型与备用模型，遇错自动故障转移
func (r *Router) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	r.mu.RLock()
	chain := append([]string{}, r.priority...)
	chain = append(chain, r.fallback...)
	r.mu.RUnlock()

	if len(chain) == 0 {
		return nil, fmt.Errorf("no AI providers registered in router")
	}

	var lastErr error
	for _, name := range chain {
		r.mu.RLock()
		provider, ok := r.providers[name]
		r.mu.RUnlock()
		if !ok {
			continue
		}

		logProviderAttempt("llm", name, "chat", req.Model, provider)
		resp, err := provider.Chat(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		model, vendor := resolveModelVendor(req.Model, provider)
		log.Printf("[AIRouter] route=%s vendor=%s model=%s chat failed: %v, falling back...", name, vendor, model, err)
	}

	return nil, fmt.Errorf("all AI providers failed, last error: %w", lastErr)
}

// ChatStream 流式对话，同样支持故障自动转移
func (r *Router) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error) {
	r.mu.RLock()
	chain := append([]string{}, r.priority...)
	chain = append(chain, r.fallback...)
	r.mu.RUnlock()

	if len(chain) == 0 {
		return nil, fmt.Errorf("no AI providers registered in router")
	}

	var lastErr error
	for _, name := range chain {
		r.mu.RLock()
		provider, ok := r.providers[name]
		r.mu.RUnlock()
		if !ok {
			continue
		}

		logProviderAttempt("llm", name, "stream", req.Model, provider)
		ch, err := provider.ChatStream(ctx, req)
		if err == nil {
			return ch, nil
		}

		lastErr = err
		model, vendor := resolveModelVendor(req.Model, provider)
		log.Printf("[AIRouter] route=%s vendor=%s model=%s stream failed: %v, falling back...", name, vendor, model, err)
	}

	return nil, fmt.Errorf("all AI providers failed for stream, last error: %w", lastErr)
}

// ChatWithTools 依序尝试主模型与备用模型，遇错自动故障转移
func (r *Router) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (*ChatWithToolsResponse, error) {
	r.mu.RLock()
	chain := append([]string{}, r.priority...)
	chain = append(chain, r.fallback...)
	r.mu.RUnlock()

	if len(chain) == 0 {
		return nil, fmt.Errorf("no AI providers registered in router")
	}

	var lastErr error
	for _, name := range chain {
		r.mu.RLock()
		provider, ok := r.providers[name]
		r.mu.RUnlock()
		if !ok {
			continue
		}

		logProviderAttempt("llm", name, "tools", req.Model, provider)
		resp, err := provider.ChatWithTools(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		model, vendor := resolveModelVendor(req.Model, provider)
		log.Printf("[AIRouter] route=%s vendor=%s model=%s tools failed: %v, falling back...", name, vendor, model, err)
	}

	return nil, fmt.Errorf("all AI providers failed for ChatWithTools, last error: %w", lastErr)
}
