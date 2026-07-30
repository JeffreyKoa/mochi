/**
 * Owner speaker verification via 3D-Speaker CAM++ ONNX (16 kHz mono).
 *
 * Model input: `x` with shape [N, T, 80] (Kaldi fbank + global mean norm).
 * Model output: `embedding` with shape [N, 192].
 *
 * Place the model at desktop/public/models/speaker/campp.onnx
 * ModelScope keyword: iic/speech_campplus_sv_zh-cn_16k-common
 */
import * as ort from 'onnxruntime-web/wasm'
import {
  applyGlobalMeanNorm,
  computeKaldiFbank,
  fbankFrameCount,
  packFbankBatch,
} from '@/services/kaldiFbank'

const ORT_BASE = 'https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/'
const MODEL_URL = '/models/speaker/campp.onnx'
const SAMPLE_RATE = 16000
const MIN_SAMPLES = SAMPLE_RATE / 2 // 0.5 s
const FBANK_DIM = 80

export interface VerifyResult {
  match: boolean
  score: number
}

export class SpeakerVerifier {
  private session: ort.InferenceSession | null = null
  private inputName = 'x'
  private outputName = 'embedding'
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

      this.inputName = this.session.inputNames[0] ?? 'x'
      this.outputName = this.session.outputNames[0] ?? 'embedding'
      this._available = true
    } catch (e) {
      console.warn('[SpeakerVerifier] init failed (fail-open)', e)
      this._available = false
    }
  }

  private pcmToFeatures(pcm: Float32Array): { data: Float32Array; numFrames: number } | null {
    if (pcm.length < MIN_SAMPLES) return null
    const numFrames = fbankFrameCount(pcm.length)
    if (numFrames <= 0) return null

    let flat = computeKaldiFbank(pcm)
    flat = applyGlobalMeanNorm(flat, numFrames, FBANK_DIM)
    const data = packFbankBatch(flat, numFrames, FBANK_DIM)
    return { data, numFrames }
  }

  async extract(pcm: Float32Array): Promise<Float32Array | null> {
    if (!this._available || !this.session) return null

    const feats = this.pcmToFeatures(pcm)
    if (!feats) return null

    // ONNX expects [N, T, 80]
    const input = new ort.Tensor('float32', feats.data, [1, feats.numFrames, FBANK_DIM])
    const feeds: Record<string, ort.Tensor> = { [this.inputName]: input }
    const out = await this.session.run(feeds)
    const tensor = out[this.outputName]
    if (!tensor?.data) return null

    const emb = tensor.data instanceof Float32Array
      ? tensor.data
      : new Float32Array(tensor.data as ArrayLike<number>)

    return SpeakerVerifier.l2Normalize(new Float32Array(emb))
  }

  async verify(
    pcm: Float32Array,
    ownerEmbedding: Float32Array,
    threshold = 0.55,
  ): Promise<VerifyResult | null> {
    const emb = await this.extract(pcm)
    if (!emb) return null
    const score = SpeakerVerifier.cosineSimilarity(emb, ownerEmbedding)
    return { match: score >= threshold, score }
  }

  /** Sliding-window max cosine score over PCM (better owner recall on long utterances). */
  async verifyMaxScore(
    pcm: Float32Array,
    ownerEmbedding: Float32Array,
    threshold: number,
    windowSec = 1.5,
    stepSec = 0.5,
  ): Promise<VerifyResult | null> {
    if (!this._available || !this.session) return null
    if (pcm.length < MIN_SAMPLES) return null

    const windowSamples = Math.floor(windowSec * SAMPLE_RATE)
    const stepSamples = Math.max(Math.floor(stepSec * SAMPLE_RATE), 1)
    let maxScore = -1
    let anyWindow = false

    for (let end = pcm.length; end >= MIN_SAMPLES; end -= stepSamples) {
      const start = Math.max(0, end - windowSamples)
      const slice = pcm.subarray(start, end)
      if (slice.length < MIN_SAMPLES) continue
      const emb = await this.extract(slice)
      if (!emb) continue
      anyWindow = true
      const score = SpeakerVerifier.cosineSimilarity(emb, ownerEmbedding)
      if (score > maxScore) maxScore = score
    }

    if (!anyWindow) {
      return this.verify(pcm, ownerEmbedding, threshold)
    }

    return { match: maxScore >= threshold, score: maxScore }
  }

  static cosineSimilarity(a: Float32Array, b: Float32Array): number {
    const n = Math.min(a.length, b.length)
    if (n === 0) return 0
    let dot = 0
    let na = 0
    let nb = 0
    for (let i = 0; i < n; i++) {
      dot += a[i] * b[i]
      na += a[i] * a[i]
      nb += b[i] * b[i]
    }
    const denom = Math.sqrt(na) * Math.sqrt(nb)
    return denom > 0 ? dot / denom : 0
  }

  static l2Normalize(v: Float32Array): Float32Array {
    let sum = 0
    for (let i = 0; i < v.length; i++) sum += v[i] * v[i]
    const norm = Math.sqrt(sum)
    if (norm <= 0) return v
    const out = new Float32Array(v.length)
    for (let i = 0; i < v.length; i++) out[i] = v[i] / norm
    return out
  }

  static averageEmbeddings(embs: Float32Array[]): Float32Array {
    if (embs.length === 0) return new Float32Array(0)
    const dim = embs[0].length
    const sum = new Float32Array(dim)
    for (const e of embs) {
      for (let i = 0; i < dim; i++) sum[i] += e[i] ?? 0
    }
    for (let i = 0; i < dim; i++) sum[i] /= embs.length
    return SpeakerVerifier.l2Normalize(sum)
  }

  destroy() {
    this.session?.release()
    this.session = null
    this._available = false
  }
}
