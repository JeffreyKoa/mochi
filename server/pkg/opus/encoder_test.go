package opus

import (
	"math"
	"testing"
)

func TestResampleLinear(t *testing.T) {
	// 22050Hz sine wave for 100ms
	srcRate := 22050
	dstRate := 48000
	samples := int(float64(srcRate) * 0.1)
	src := make([]int16, samples)
	for i := 0; i < samples; i++ {
		src[i] = int16(3000 * math.Sin(2*math.Pi*440*float64(i)/float64(srcRate)))
	}

	resampled := ResampleLinear(src, srcRate, dstRate)
	expectedLen := int(float64(samples) * float64(dstRate) / float64(srcRate))

	if math.Abs(float64(len(resampled)-expectedLen)) > 2 {
		t.Errorf("expected len ~%d, got %d", expectedLen, len(resampled))
	}
}

func TestEncoder_EncodeChunk(t *testing.T) {
	enc, err := NewEncoder(22050, 24000)
	if err != nil {
		t.Skipf("opus encoder unavailable: %v", err)
	}

	// 40ms of silence at 22050Hz → should yield at least one 20ms Opus frame after resample.
	samples := 22050 * 40 / 1000
	pcm := make([]byte, samples*2)
	frames, err := enc.EncodeChunk(pcm)
	if err != nil {
		t.Fatalf("EncodeChunk: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("expected at least one opus frame")
	}
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Fatalf("frame %d is empty", i)
		}
	}
}
