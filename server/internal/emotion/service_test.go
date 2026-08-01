package emotion

import "testing"

func TestMergeAcousticHint_neutralTextSadVoice(t *testing.T) {
	cached := Hint{UserMood: "neutral", Intent: "chat", Temperature: 0.85}
	quick := QuickDetect("我没事")
	if quick.UserMood != "neutral" {
		t.Fatalf("expected neutral quick detect, got %s", quick.UserMood)
	}

	acoustic := AcousticHint{Mood: "sad", Confidence: 0.85, Label: "sad"}
	merged := MergeAcousticHint(cached, quick, acoustic, 0.65)

	if merged.UserMood != "sad" {
		t.Errorf("UserMood = %s, want sad", merged.UserMood)
	}
	if !merged.NeedsEmpathy {
		t.Error("expected NeedsEmpathy true")
	}
	if merged.Intent != "vent" {
		t.Errorf("Intent = %s, want vent", merged.Intent)
	}
}

func TestMergeAcousticHint_ventTextWins(t *testing.T) {
	quick := QuickDetect("我真的好烦啊")
	if !quick.NeedsEmpathy {
		t.Fatal("test message should trigger vent quick detect")
	}
	acoustic := AcousticHint{Mood: "happy", Confidence: 0.9}
	merged := MergeAcousticHint(Hint{}, quick, acoustic, 0.65)
	if merged.UserMood != "stressed" {
		t.Errorf("text vent should win, got %s", merged.UserMood)
	}
}

func TestMergeAcousticHint_lowConfidenceIgnored(t *testing.T) {
	quick := QuickDetect("我没事")
	acoustic := AcousticHint{Mood: "sad", Confidence: 0.4}
	merged := MergeAcousticHint(Hint{}, quick, acoustic, 0.65)
	if merged.UserMood != "neutral" {
		t.Errorf("low confidence should not override, got %s", merged.UserMood)
	}
}
