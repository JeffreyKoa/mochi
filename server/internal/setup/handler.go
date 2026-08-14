package setup

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/mochi-ai/server/internal/config"
)

// Handler 本地部署 setup API（写 config.yaml modules 段）。
type Handler struct {
	cfgPath string
	onSave  func(*config.Config) // 可选：保存后回调（测试用）
}

// NewHandler 创建 setup handler；cfgPath 为 config.yaml 绝对路径。
func NewHandler(cfgPath string) *Handler {
	return &Handler{cfgPath: cfgPath}
}

type modulePatch struct {
	Enabled  *bool  `json:"enabled"`
	Provider string `json:"provider"`
}

type updateModulesRequest struct {
	ASR     *modulePatch `json:"asr"`
	TTS     *modulePatch `json:"tts"`
	LLM     *modulePatch `json:"llm"`
	Vision  *modulePatch `json:"vision"`
	Emotion *modulePatch `json:"emotion"`
	// 可选：远程 LLM Key（写入 ai.api_key，仅本地自建场景）
	APIKey string `json:"api_key,omitempty"`
}

// RegisterRoutes 注册需鉴权的路由。
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/setup/modules", h.GetModules)
	r.PUT("/setup/modules", h.UpdateModules)
}

// GetModules 返回当前 modules 配置（与 public/modules 类似但可含更多字段）。
func (h *Handler) GetModules(c *gin.Context) {
	cfg, err := config.Load()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"modules": cfg.PublicModules(),
		"note":    "修改后需 restart-backend 使 sidecar 生效",
	})
}

// UpdateModules 更新 config.yaml 中 modules（及可选 ai.api_key）。
func (h *Handler) UpdateModules(c *gin.Context) {
	var req updateModulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	path := h.cfgPath
	if path == "" {
		path = config.LoadedPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "read config: " + err.Error()})
		return
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parse config: " + err.Error()})
		return
	}
	modules, _ := root["modules"].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		root["modules"] = modules
	}
	applyPatch(modules, "asr", req.ASR)
	applyPatch(modules, "tts", req.TTS)
	applyPatch(modules, "llm", req.LLM)
	applyPatch(modules, "vision", req.Vision)
	applyPatch(modules, "emotion", req.Emotion)
	if req.APIKey != "" {
		ai, _ := root["ai"].(map[string]interface{})
		if ai == nil {
			ai = map[string]interface{}{}
			root["ai"] = ai
		}
		ai["api_key"] = req.APIKey
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal config: " + err.Error()})
		return
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write config: " + err.Error()})
		return
	}
	reloaded, err := config.Load()
	if err == nil && h.onSave != nil {
		h.onSave(reloaded)
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"restart_hint":  "请运行 scripts/restart-backend.ps1 使 sidecar 变更生效",
		"modules":       reloaded.PublicModules(),
	})
}

func applyPatch(modules map[string]interface{}, name string, patch *modulePatch) {
	if patch == nil {
		return
	}
	entry, _ := modules[name].(map[string]interface{})
	if entry == nil {
		entry = map[string]interface{}{}
		modules[name] = entry
	}
	if patch.Enabled != nil {
		entry["enabled"] = *patch.Enabled
	}
	if patch.Provider != "" {
		entry["provider"] = patch.Provider
	}
}
