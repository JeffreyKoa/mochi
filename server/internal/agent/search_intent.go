package agent

import "strings"

// NeedsWebSearch returns true when the user message likely needs up-to-date web information.
func NeedsWebSearch(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)

	keywords := []string{
		"搜索", "查一下", "查下", "帮我查", "搜一下", "搜下", "联网", "上网查",
		"最新", "今天", "今日", "现在", "当前", "实时", "最近", "刚刚", "刚才",
		"新闻", "头条", "热点", "天气", "台风", "地震", "暴雨", "气温",
		"股价", "股票", "汇率", "金价", "油价",
		"多少钱", "价格", "售价", "发布会", "政策",
		"怎么回事", "发生了什么", "什么情况",
		"总结", "梳理", "概括", "整理一下",
		"search", "look up", "latest", "news", "weather", "today",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
