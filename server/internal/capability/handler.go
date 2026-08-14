package capability

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/config"
)

// Handler 暴露硬件能力与 setup 状态 API。
type Handler struct {
	cfg *config.Config
}

// NewHandler 创建 capability HTTP handler。
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg}
}

// RegisterRoutes 注册公开路由（无鉴权）。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/public/capability", h.GetCapability)
	r.GET("/api/v1/public/setup-status", h.GetSetupStatus)
	r.GET("/api/v1/public/modules", h.GetModules)
}

// GetCapability 硬件摘要 + 各模块能力评级。
func (h *Handler) GetCapability(c *gin.Context) {
	hw := Probe()
	report := Evaluate(hw, h.cfg)
	c.JSON(http.StatusOK, report)
}

// GetSetupStatus sidecar 健康与模型就绪状态。
func (h *Handler) GetSetupStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	report := ProbeSetup(ctx, h.cfg)
	c.JSON(http.StatusOK, report)
}

// GetModules 各模块 enabled / provider / 能力状态（供设置页）。
func (h *Handler) GetModules(c *gin.Context) {
	hw := Probe()
	capReport := Evaluate(hw, h.cfg)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	setupReport := ProbeSetup(ctx, h.cfg)

	setupByName := map[ModuleName]SidecarStatus{}
	for _, s := range setupReport.Modules {
		setupByName[s.Name] = s
	}

	type moduleView struct {
		Name                 ModuleName `json:"name"`
		Enabled              bool       `json:"enabled"`
		Provider             string     `json:"provider"`
		CapabilityStatus     Status     `json:"capability_status"`
		CapabilityReason     string     `json:"capability_reason,omitempty"`
		SidecarHealthy       *bool      `json:"sidecar_healthy,omitempty"`
		ModelReady           *bool      `json:"model_ready,omitempty"`
		RequiresLocalSidecar bool       `json:"requires_local_sidecar"`
	}
	out := make([]moduleView, 0, len(capReport.Modules))
	for _, m := range capReport.Modules {
		v := moduleView{
			Name:                 m.Name,
			Enabled:              m.Enabled,
			Provider:             m.Provider,
			CapabilityStatus:     m.Status,
			CapabilityReason:     m.Reason,
			RequiresLocalSidecar: m.RequiresLocalSidecar,
		}
		if st, ok := setupByName[m.Name]; ok && m.Enabled {
			h := st.Healthy
			r := st.ModelReady
			v.SidecarHealthy = &h
			v.ModelReady = &r
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, gin.H{"modules": out})
}
