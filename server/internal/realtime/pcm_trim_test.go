package realtime

import "testing"

func TestTrimPCMForASR(t *testing.T) {
	short := make([]byte, 1000)
	if got := trimPCMForASR(short); len(got) != len(short) {
		t.Fatalf("short pcm should not be trimmed: got %d want %d", len(got), len(short))
	}

	long := make([]byte, maxBatchASRBytes+8000)
	for i := range long {
		long[i] = byte(i % 256)
	}
	got := trimPCMForASR(long)
	if len(got) != maxBatchASRBytes {
		t.Fatalf("trimPCMForASR len=%d want %d", len(got), maxBatchASRBytes)
	}
	if got[0] != long[len(long)-maxBatchASRBytes] {
		t.Fatal("trimPCMForASR should keep tail segment")
	}
}

func TestTrimPCMForEmotion(t *testing.T) {
	long := make([]byte, maxEmotionPCMBytes+4000)
	got := trimPCMForEmotion(long)
	if len(got) != maxEmotionPCMBytes {
		t.Fatalf("trimPCMForEmotion len=%d want %d", len(got), maxEmotionPCMBytes)
	}
}
