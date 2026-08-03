package vision

import "testing"

func TestShouldSkipTier1_WeatherSkip(t *testing.T) {
	topics := []string{"weather", "news", "time", "translate", "calculate"}
	if !ShouldSkipTier1("深圳天气怎么样", topics, true) {
		t.Fatal("expected skip for pure weather query")
	}
}

func TestShouldSkipTier1_DeicticExempt(t *testing.T) {
	topics := []string{"weather"}
	if ShouldSkipTier1("看看窗外天气怎么样", topics, true) {
		t.Fatal("deictic cue should exempt skip")
	}
	if ShouldSkipTier1("这上面的天气预报是哪个城市", topics, true) {
		t.Fatal("deictic 这/上面 should exempt")
	}
}

func TestShouldSkipTier1_ClothingExempt(t *testing.T) {
	topics := []string{"weather"}
	if ShouldSkipTier1("我穿这身适合今天吗", topics, true) {
		t.Fatal("穿/这 should exempt even without weather keyword")
	}
}

func TestHasDeicticVisualCue(t *testing.T) {
	if !HasDeicticVisualCue("这是什么") {
		t.Fatal("expected deictic")
	}
	if HasDeicticVisualCue("今天好累") {
		t.Fatal("expected no deictic")
	}
}
