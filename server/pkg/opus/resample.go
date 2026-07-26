package opus

// ResampleLinear resamples int16 audio from srcRate to dstRate using linear interpolation.
func ResampleLinear(src []int16, srcRate, dstRate int) []int16 {
	if srcRate == dstRate || len(src) == 0 {
		return src
	}
	ratio := float64(srcRate) / float64(dstRate)
	outLen := int(float64(len(src)) / ratio)
	out := make([]int16, outLen)

	for i := 0; i < outLen; i++ {
		srcIdx := float64(i) * ratio
		idx0 := int(srcIdx)
		idx1 := idx0 + 1
		if idx1 >= len(src) {
			idx1 = len(src) - 1
		}
		frac := srcIdx - float64(idx0)
		sample := float64(src[idx0])*(1.0-frac) + float64(src[idx1])*frac
		out[i] = int16(sample)
	}
	return out
}
