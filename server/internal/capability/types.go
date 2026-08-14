package capability

// ModuleName 标识 AI 模型模块。
type ModuleName string

const (
	ModuleASR     ModuleName = "asr"
	ModuleTTS     ModuleName = "tts"
	ModuleLLM     ModuleName = "llm"
	ModuleVision  ModuleName = "vision"
	ModuleEmotion ModuleName = "emotion"
)

// Status 模块运行能力或开关状态。
type Status string

const (
	StatusDisabled     Status = "disabled"
	StatusOK           Status = "ok"
	StatusDegraded     Status = "degraded"
	StatusUnsupported  Status = "unsupported"
	StatusRemoteReady  Status = "remote_ready"
	StatusRemoteMissing Status = "remote_missing"
)

// GPUInfo 本机 GPU 摘要。
type GPUInfo struct {
	Present     bool   `json:"present"`
	Name        string `json:"name,omitempty"`
	VRAMTotalMB int64  `json:"vram_total_mb,omitempty"`
}

// HardwareSnapshot 硬件探测结果（不含密钥）。
type HardwareSnapshot struct {
	CPUCores        int    `json:"cpu_cores"`
	CPUModel        string `json:"cpu_model,omitempty"`
	RAMTotalMB      int64  `json:"ram_total_mb"`
	RAMAvailableMB  int64  `json:"ram_available_mb"`
	DiskFreeMB      int64  `json:"disk_free_mb"`
	GPU             GPUInfo `json:"gpu"`
}

// ModuleReport 单模块能力评级。
type ModuleReport struct {
	Name                 ModuleName `json:"name"`
	Enabled              bool       `json:"enabled"`
	Provider             string     `json:"provider"`
	Status               Status     `json:"status"`
	Reason               string     `json:"reason,omitempty"`
	RequiresLocalSidecar bool       `json:"requires_local_sidecar"`
}

// Report 完整能力报告。
type Report struct {
	Hardware HardwareSnapshot `json:"hardware"`
	Modules  []ModuleReport   `json:"modules"`
}

// SidecarStatus sidecar 健康与模型就绪状态。
type SidecarStatus struct {
	Name        ModuleName `json:"name"`
	Enabled     bool       `json:"enabled"`
	URL         string     `json:"url,omitempty"`
	Healthy     bool       `json:"healthy"`
	ModelReady  bool       `json:"model_ready"`
	Status      Status     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
}

// SetupReport 启动/setup 聚合状态。
type SetupReport struct {
	Modules []SidecarStatus `json:"modules"`
}
