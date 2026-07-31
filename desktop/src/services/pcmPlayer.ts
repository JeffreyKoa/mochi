import { emitLipSync } from './voice'

/** PCM int16 LE mono player for streaming TTS (22050Hz default) */
export class PCMPlayer {
  private context: AudioContext | null = null
  private nextTime = 0
  private sampleRate: number
  private active = false
  private pending = 0
  private onIdle: (() => void) | null = null

  constructor(sampleRate = 22050) {
    this.sampleRate = sampleRate
  }

  private async ensureContext() {
    if (!this.context) {
      this.context = new AudioContext({ sampleRate: this.sampleRate })
    }
    if (this.context.state === 'suspended') {
      await this.context.resume()
    }
  }

  async enqueue(pcm: ArrayBuffer) {
    await this.ensureContext()
    if (!this.context || pcm.byteLength < 2) return

    const view = new DataView(pcm)
    const samples = pcm.byteLength / 2
    const buffer = this.context.createBuffer(1, samples, this.sampleRate)
    const channel = buffer.getChannelData(0)
    let sum = 0
    for (let i = 0; i < samples; i++) {
      const val = view.getInt16(i * 2, true) / 32768
      channel[i] = val
      sum += Math.abs(val)
    }

    const avgVol = Math.min(1, (sum / samples) * 3)
    emitLipSync(avgVol)

    const source = this.context.createBufferSource()
    source.buffer = buffer
    source.connect(this.context.destination)

    const now = this.context.currentTime
    if (this.nextTime < now) this.nextTime = now
    source.start(this.nextTime)
    this.nextTime += buffer.duration

    this.active = true
    this.pending++
    source.onended = () => {
      this.pending--
      if (this.pending <= 0) {
        this.pending = 0
        this.active = false
        emitLipSync(0)
        this.onIdle?.()
      }
    }
  }

  markDone(onIdle?: () => void) {
    this.onIdle = onIdle ?? null
    if (!this.active || this.pending <= 0) {
      this.onIdle?.()
      this.onIdle = null
    }
  }

  stop() {
    this.pending = 0
    this.active = false
    this.onIdle = null
    emitLipSync(0)
    if (this.context) {
      void this.context.close()
      this.context = null
    }
    this.nextTime = 0
  }
}

export function base64ToArrayBuffer(b64: string): ArrayBuffer {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}
