package dashscope

import "testing"

func TestResolveWSURL(t *testing.T) {
	if got := ResolveWSURL("", "", ""); got != defaultWSURL {
		t.Fatalf("default: got %q", got)
	}
	if got := ResolveWSURL("wss://custom.example/ws", "ws-1", "cn-beijing"); got != "wss://custom.example/ws" {
		t.Fatalf("custom override: got %q", got)
	}
	want := "wss://ws-abc123.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference"
	if got := ResolveWSURL("", "ws-abc123", "cn-beijing"); got != want {
		t.Fatalf("workspace: got %q want %q", got, want)
	}
	if got := ResolveWSURL("", "ws-abc123", ""); got != want {
		t.Fatalf("default region: got %q want %q", got, want)
	}
}
