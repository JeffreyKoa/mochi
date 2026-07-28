/**
 * YAMNet sound-event classifier (521-class AudioSet) via ONNX.
 *
 * Place exported YAMNet ONNX at desktop/public/models/audio/yamnet.onnx
 * (not wired into the realtime pipeline yet — standalone module).
 */
import * as ort from 'onnxruntime-web/wasm'

const ORT_BASE = 'https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/'
const MODEL_URL = '/models/audio/yamnet.onnx'
const WINDOW_SAMPLES = 15360 // 0.96 s @ 16 kHz

/** YAMNet AudioSet indices that represent human speech or vocalization. */
const SPEECH_INDICES = new Set([
  0, // Speech
  1, // Child speech, kid speaking
  2, // Conversation
  3, // Narration, monologue
  4, // Babbling
  5, // Speech synthesizer
  8, // Shout
  9, // Bellow
  10, // Whoop
  11, // Yell
  12, // Children shouting
  13, // Screaming
  14, // Whispering
])

export interface ClassifyResult {
  speechScore: number
  topIndex: number
  topScore: number
  topLabel: string
}

const INDEX_LABELS: Record<number, string> = {
  0: 'Speech',
  1: 'Child speech',
  2: 'Conversation',
  3: 'Narration/monologue',
  4: 'Babbling',
  5: 'Speech synthesizer',
  8: 'Shout',
  9: 'Bellow',
  10: 'Whoop',
  11: 'Yell',
  12: 'Children shouting',
  13: 'Screaming',
  14: 'Whispering',
}

export class SoundEventClassifier {
  private session: ort.InferenceSession | null = null
  private inputName = ''
  _available = false

  get available(): boolean {
    return this._available
  }

  async init(): Promise<void> {
    try {
      ort.env.logLevel = 'error'
      ort.env.wasm.wasmPaths = ORT_BASE

      const res = await fetch(MODEL_URL)
      if (!res.ok) throw new Error(`model fetch ${res.status}`)
      const buf = await res.arrayBuffer()

      this.session = await ort.InferenceSession.create(buf, {
        executionProviders: ['wasm'],
      })
      this.inputName = this.session.inputNames[0] ?? 'input'
      this._available = true
    } catch (e) {
      console.warn('[SoundEventClassifier] init failed (fail-open)', e)
      this._available = false
    }
  }

  async classify(pcm: Float32Array): Promise<ClassifyResult | null> {
    if (!this._available || !this.session) return null

    const window = new Float32Array(WINDOW_SAMPLES)
    const copyLen = Math.min(pcm.length, WINDOW_SAMPLES)
    window.set(pcm.subarray(0, copyLen))

    const input = new ort.Tensor('float32', window, [1, WINDOW_SAMPLES])
    const feeds: Record<string, ort.Tensor> = { [this.inputName]: input }
    const out = await this.session.run(feeds)
    const tensor = out[this.session.outputNames[0]]
    if (!tensor?.data) return null

    const scores =
      tensor.data instanceof Float32Array
        ? tensor.data
        : new Float32Array(tensor.data as ArrayLike<number>)

    let topIndex = 0
    let topScore = scores[0] ?? 0
    let speechScore = 0

    for (let i = 0; i < scores.length; i++) {
      const s = scores[i]
      if (s > topScore) {
        topScore = s
        topIndex = i
      }
      if (SPEECH_INDICES.has(i)) {
        speechScore = Math.max(speechScore, s)
      }
    }

    return {
      speechScore,
      topIndex,
      topScore,
      topLabel: INDEX_LABELS[topIndex] ?? `class_${topIndex}`,
    }
  }

  destroy() {
    this.session?.release()
    this.session = null
    this._available = false
  }
}

export function isSpeechLike(label: string, threshold = 0.3, speechScore?: number): boolean {
  if (speechScore !== undefined && speechScore >= threshold) return true
  const lower = label.toLowerCase()
  return (
    lower.includes('speech') ||
    lower.includes('conversation') ||
    lower.includes('narration') ||
    lower.includes('monologue') ||
    lower.includes('shout') ||
    lower.includes('scream') ||
    lower.includes('whisper') ||
    lower.includes('yell') ||
    lower.includes('babbl')
  )
}
