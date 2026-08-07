package realtime

// PCM 截断工具：降低长缓冲上的批量 ASR / 声学识别延迟。

// maxBatchASRBytes：批量 ASR 最多识别最近 6 秒，避免 20s+ 缓冲识别耗时 10s+。
const maxBatchASRBytes = 16000 * 2 * 6

// maxEmotionPCMBytes：emotion2vec 只送最近 5 秒（句末情绪更敏感，且显著降 SER 耗时）。
const maxEmotionPCMBytes = 16000 * 2 * 5

// trimPCMForASR 截取 PCM 尾部用于批量 ASR。
func trimPCMForASR(pcm []byte) []byte {
	if len(pcm) <= maxBatchASRBytes {
		return pcm
	}
	return pcm[len(pcm)-maxBatchASRBytes:]
}

// trimPCMForEmotion 截取 PCM 尾部用于 emotion2vec 声学情绪识别。
func trimPCMForEmotion(pcm []byte) []byte {
	if len(pcm) <= maxEmotionPCMBytes {
		return pcm
	}
	return pcm[len(pcm)-maxEmotionPCMBytes:]
}
