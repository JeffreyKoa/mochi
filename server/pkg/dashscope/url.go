package dashscope

import "fmt"

const defaultWSURL = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

// ResolveWSURL picks the DashScope WebSocket endpoint.
// Priority: explicit wsURL > workspace region URL > default global URL.
func ResolveWSURL(wsURL, workspaceID, region string) string {
	if wsURL != "" {
		return wsURL
	}
	if workspaceID != "" {
		if region == "" {
			region = "cn-beijing"
		}
		return fmt.Sprintf("wss://%s.%s.maas.aliyuncs.com/api-ws/v1/inference", workspaceID, region)
	}
	return defaultWSURL
}
