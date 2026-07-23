/**
 * 16kHz mono PCM capture, 20ms per chunk (640 bytes)
 */
import workletUrl from './pcm-worklet.processor.ts?url'

const TARGET_RATE = 16000
const CHUNK_SAMPLES = 320 // 20ms @ 16kHz

type ChunkHandler = (pcm: ArrayBuffer, seq: number) => void

export class PCMCapture {
  private stream: MediaStream | null = null
  private context: AudioContext | null = null
  private worklet: AudioWorkletNode | null = null
  private processor: ScriptProcessorNode | null = null
  private active = false
  private onChunk: ChunkHandler | null = null

  async start(onChunk: ChunkHandler): Promise<void> {
    if (this.active) return
    this.onChunk = onChunk

    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
    })

    const track = this.stream.getAudioTracks()[0]
    if (track) {
      const settings = track.getSettings()
      if (settings.echoCancellation === false) {
        console.warn('[pcmCapture] echoCancellation unavailable — barge-in may be less reliable')
      }
      if (settings.noiseSuppression === false) {
        console.warn('[pcmCapture] noiseSuppression unavailable')
      }
    }

    this.context = new AudioContext()
    if (this.context.state === 'suspended') {
      await this.context.resume()
    }

    const source = this.context.createMediaStreamSource(this.stream)
    this.active = true

    try {
      await this.startWorklet(source)
    } catch {
      this.startScriptProcessor(source)
    }
  }

  private async startWorklet(source: MediaStreamAudioSourceNode) {
    if (!this.context) throw new Error('no context')

    await this.context.audioWorklet.addModule(workletUrl)

    this.worklet = new AudioWorkletNode(this.context, 'pcm-worklet')
    this.worklet.port.onmessage = (e: MessageEvent<{ pcm: ArrayBuffer; seq: number }>) => {
      if (!this.active || !this.onChunk) return
      this.onChunk(e.data.pcm, e.data.seq)
    }

    source.connect(this.worklet)
  }

  private startScriptProcessor(source: MediaStreamAudioSourceNode) {
    if (!this.context) return

    this.processor = this.context.createScriptProcessor(4096, 1, 1)
    const silent = this.context.createGain()
    silent.gain.value = 0

    let seq = 0
    let buffer = new Float32Array(0)
    const sourceRate = this.context.sampleRate

    this.processor.onaudioprocess = (e) => {
      if (!this.active || !this.onChunk) return

      const input = e.inputBuffer.getChannelData(0)
      const output = e.outputBuffer.getChannelData(0)
      output.set(input)

      const resampled = resample(input, sourceRate, TARGET_RATE)
      const merged = new Float32Array(buffer.length + resampled.length)
      merged.set(buffer)
      merged.set(resampled, buffer.length)
      buffer = merged

      while (buffer.length >= CHUNK_SAMPLES) {
        const slice = buffer.slice(0, CHUNK_SAMPLES)
        buffer = buffer.slice(CHUNK_SAMPLES)
        seq++
        this.onChunk(floatTo16LE(slice), seq)
      }
    }

    source.connect(this.processor)
    this.processor.connect(silent)
    silent.connect(this.context.destination)
  }

  async stop(): Promise<void> {
    this.active = false
    this.onChunk = null
    this.worklet?.port.close()
    this.worklet?.disconnect()
    this.worklet = null
    this.processor?.disconnect()
    this.processor = null
    if (this.context) {
      await this.context.close()
      this.context = null
    }
    this.stream?.getTracks().forEach((t) => t.stop())
    this.stream = null
  }

  get isActive() {
    return this.active
  }
}

function resample(input: Float32Array, fromRate: number, toRate: number): Float32Array {
  if (fromRate === toRate) return input
  const ratio = fromRate / toRate
  const outLen = Math.max(1, Math.floor(input.length / ratio))
  const out = new Float32Array(outLen)
  for (let i = 0; i < outLen; i++) {
    const srcIdx = i * ratio
    const idx = Math.floor(srcIdx)
    const frac = srcIdx - idx
    const s0 = input[idx] ?? 0
    const s1 = input[Math.min(idx + 1, input.length - 1)] ?? s0
    out[i] = s0 + (s1 - s0) * frac
  }
  return out
}

function floatTo16LE(samples: Float32Array): ArrayBuffer {
  const buf = new ArrayBuffer(samples.length * 2)
  const view = new DataView(buf)
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]))
    view.setInt16(i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true)
  }
  return buf
}

export function arrayBufferToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  return btoa(binary)
}

/** Peak level 0..1 for UI meter */
export function pcmPeakLevel(pcm: ArrayBuffer): number {
  const view = new DataView(pcm)
  let peak = 0
  for (let i = 0; i < view.byteLength; i += 2) {
    const s = Math.abs(view.getInt16(i, true))
    if (s > peak) peak = s
  }
  return peak / 32768
}

/** Boost quiet mic input so server VAD can detect speech */
export function amplifyPCM(pcm: ArrayBuffer, targetPeak = 0.3): ArrayBuffer {
  const view = new DataView(pcm)
  let peak = 0
  for (let i = 0; i < view.byteLength; i += 2) {
    peak = Math.max(peak, Math.abs(view.getInt16(i, true)))
  }
  if (peak >= targetPeak * 32768 * 0.5) return pcm

  const gain = Math.min(6, (targetPeak * 32768) / Math.max(peak, 80))
  const out = new ArrayBuffer(pcm.byteLength)
  const outView = new DataView(out)
  for (let i = 0; i < view.byteLength; i += 2) {
    const amplified = Math.round(view.getInt16(i, true) * gain)
    outView.setInt16(i, Math.max(-32768, Math.min(32767, amplified)), true)
  }
  return out
}