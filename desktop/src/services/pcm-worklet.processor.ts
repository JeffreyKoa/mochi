const TARGET_RATE = 16000
const CHUNK_SAMPLES = 320

class PCMWorkletProcessor extends AudioWorkletProcessor {
  private pending: number[] = []
  private seq = 0
  private readonly step: number

  constructor() {
    super()
    this.step = sampleRate / TARGET_RATE
  }

  process(inputs: Float32Array[][], _outputs: Float32Array[][], _parameters: Record<string, Float32Array>) {
    const input = inputs[0]?.[0]
    if (!input?.length) return true

    const outCount = Math.floor(input.length / this.step)
    for (let i = 0; i < outCount; i++) {
      const srcIdx = i * this.step
      const idx = Math.floor(srcIdx)
      const frac = srcIdx - idx
      const s0 = input[idx] ?? 0
      const s1 = input[Math.min(idx + 1, input.length - 1)] ?? s0
      this.pending.push(s0 + (s1 - s0) * frac)
    }

    while (this.pending.length >= CHUNK_SAMPLES) {
      const slice = this.pending.splice(0, CHUNK_SAMPLES)
      const pcm = new ArrayBuffer(CHUNK_SAMPLES * 2)
      const view = new DataView(pcm)
      for (let i = 0; i < CHUNK_SAMPLES; i++) {
        const s = Math.max(-1, Math.min(1, slice[i]))
        view.setInt16(i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true)
      }
      this.seq++
      this.port.postMessage({ pcm, seq: this.seq }, [pcm])
    }

    return true
  }
}

registerProcessor('pcm-worklet', PCMWorkletProcessor)

export {}
