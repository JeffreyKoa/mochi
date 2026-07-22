/**
 * 16kHz 单声道 WAV 录音（腾讯云 ASR 兼容）
 */
export class VoiceRecorder {
  private stream: MediaStream | null = null
  private context: AudioContext | null = null
  private processor: ScriptProcessorNode | null = null
  private chunks: Float32Array[] = []
  private recording = false

  async start(): Promise<void> {
    if (this.recording) return

    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
    })

    this.context = new AudioContext({ sampleRate: 16000 })
    const source = this.context.createMediaStreamSource(this.stream)
    this.processor = this.context.createScriptProcessor(4096, 1, 1)
    this.chunks = []

    this.processor.onaudioprocess = (e) => {
      if (!this.recording) return
      const input = e.inputBuffer.getChannelData(0)
      this.chunks.push(new Float32Array(input))
    }

    source.connect(this.processor)
    this.processor.connect(this.context.destination)
    this.recording = true
  }

  async stop(): Promise<Blob> {
    this.recording = false

    if (this.processor) {
      this.processor.disconnect()
      this.processor = null
    }
    if (this.context) {
      await this.context.close()
      this.context = null
    }
    if (this.stream) {
      this.stream.getTracks().forEach((t) => t.stop())
      this.stream = null
    }

    const samples = mergeFloat32(this.chunks)
    return encodeWAV(samples, 16000)
  }

  get isRecording() {
    return this.recording
  }
}

function mergeFloat32(chunks: Float32Array[]): Float32Array {
  const total = chunks.reduce((n, c) => n + c.length, 0)
  const out = new Float32Array(total)
  let offset = 0
  for (const c of chunks) {
    out.set(c, offset)
    offset += c.length
  }
  return out
}

function encodeWAV(samples: Float32Array, sampleRate: number): Blob {
  const buffer = new ArrayBuffer(44 + samples.length * 2)
  const view = new DataView(buffer)

  writeString(view, 0, 'RIFF')
  view.setUint32(4, 36 + samples.length * 2, true)
  writeString(view, 8, 'WAVE')
  writeString(view, 12, 'fmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, sampleRate, true)
  view.setUint32(28, sampleRate * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  writeString(view, 36, 'data')
  view.setUint32(40, samples.length * 2, true)

  let offset = 44
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]))
    view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true)
    offset += 2
  }

  return new Blob([buffer], { type: 'audio/wav' })
}

function writeString(view: DataView, offset: number, str: string) {
  for (let i = 0; i < str.length; i++) {
    view.setUint8(offset + i, str.charCodeAt(i))
  }
}

const activeAudios: HTMLAudioElement[] = []

export function stopAllPlayback() {
  for (const audio of activeAudios) {
    audio.pause()
    audio.src = ''
  }
  activeAudios.length = 0
}

export function playBase64Audio(base64: string, format = 'mp3'): Promise<void> {
  return new Promise((resolve, reject) => {
    const mime = format === 'mp3' ? 'audio/mpeg' : `audio/${format}`
    const binary = atob(base64)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    const blob = new Blob([bytes], { type: mime })
    const url = URL.createObjectURL(blob)
    const audio = new Audio(url)
    activeAudios.push(audio)
    const cleanup = () => {
      URL.revokeObjectURL(url)
      const idx = activeAudios.indexOf(audio)
      if (idx >= 0) activeAudios.splice(idx, 1)
    }
    audio.onended = () => {
      cleanup()
      resolve()
    }
    audio.onerror = () => {
      cleanup()
      reject(new Error('audio playback failed'))
    }
    audio.play().catch((e) => {
      cleanup()
      reject(e)
    })
  })
}
