package modelmeta

import "testing"

func TestInferVendorFromAPIBase(t *testing.T) {
	cases := []struct {
		base   string
		vendor string
	}{
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", VendorAliyunDashScope},
		{"wss://llm-xxx.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference", VendorAliyunDashScope},
		{"https://ark.cn-beijing.volces.com/api/v3", VendorVolcengine},
		{"https://api.openai.com/v1", VendorOpenAI},
		{"https://example.com/v1", VendorCompatibleAPI},
	}
	for _, c := range cases {
		if got := InferVendorFromAPIBase(c.base); got != c.vendor {
			t.Errorf("InferVendorFromAPIBase(%q) = %q, want %q", c.base, got, c.vendor)
		}
	}
}
