/**
 * 主人人脸识别（P2）：客户端 InsightFace 风格 ONNX embedding。
 *
 * 识别模型：desktop/public/models/face/rec.onnx
 *   输入 [1, 3, 112, 112]，像素 (x - 127.5) / 128
 *   输出 512 维 embedding（维度以模型为准）
 *
 * 可选检测模型：desktop/public/models/face/det.onnx
 *   未放置时退化为画面中心 square crop（录入时正对镜头即可）
 *
 * ModelScope / InsightFace buffalo_l 可导出 w600k_r50.onnx 作 rec。
 */
import * as ort from 'onnxruntime-web/wasm'
import { faceDetector, type FaceCropRect } from '@/services/faceDet'

const ORT_BASE = 'https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/'
const REC_MODEL_URL = '/models/face/rec.onnx'
const INPUT_SIZE = 112
const MIN_FACE_SIDE = 48
/** 低于此分视为「看不清脸」，不算 detected（中心 crop 误检）。 */
export const MIN_FACE_DETECT_SCORE = 0.22
/** 高于此分且非 match 才视为「像他人脸」（避免 0.25~0.41 灰区误标 unknown）。 */
export const MIN_FACE_UNKNOWN_SCORE = 0.32
/** 唤醒/拒答：人脸高分即使声纹未过也视为主人。 */
export const FACE_OWNER_BOOST_SCORE = 0.38

export interface FaceVerifyResult {
  match: boolean
  score: number
  /** 帧内是否检测到可用人脸区域 */
  detected: boolean
}

/** JPEG → ImageData（浏览器解码） */
async function jpegToImageData(jpeg: ArrayBuffer): Promise<ImageData | null> {
  try {
    const blob = new Blob([jpeg], { type: 'image/jpeg' })
    const bitmap = await createImageBitmap(blob)
    const canvas = document.createElement('canvas')
    canvas.width = bitmap.width
    canvas.height = bitmap.height
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      bitmap.close()
      return null
    }
    ctx.drawImage(bitmap, 0, 0)
    bitmap.close()
    return ctx.getImageData(0, 0, canvas.width, canvas.height)
  } catch (e) {
    console.warn('[FaceVerifier] jpeg decode failed', e)
    return null
  }
}

/** 双线性缩放到 target×target RGB float32，归一化 (x - 127.5) / 128 */
function cropAndNormalize(
  img: ImageData,
  x: number,
  y: number,
  size: number,
): Float32Array | null {
  const side = Math.floor(size)
  if (side < MIN_FACE_SIDE) return null

  const canvas = document.createElement('canvas')
  canvas.width = INPUT_SIZE
  canvas.height = INPUT_SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx) return null

  const tmp = document.createElement('canvas')
  tmp.width = img.width
  tmp.height = img.height
  const tctx = tmp.getContext('2d')
  if (!tctx) return null
  tctx.putImageData(img, 0, 0)

  ctx.drawImage(tmp, x, y, side, side, 0, 0, INPUT_SIZE, INPUT_SIZE)
  const cropped = ctx.getImageData(0, 0, INPUT_SIZE, INPUT_SIZE)
  const out = new Float32Array(3 * INPUT_SIZE * INPUT_SIZE)
  const data = cropped.data
  let o = 0
  for (let c = 0; c < 3; c++) {
    for (let i = c; i < data.length; i += 4) {
      out[o++] = (data[i] - 127.5) / 128
    }
  }
  return out
}

export class FaceVerifier {
  private recSession: ort.InferenceSession | null = null
  private recInputName = 'input.1'
  private recOutputName = '683'
  _available = false
  /** det.onnx 是否可用（与 rec 独立） */
  _detAvailable = false

  get available(): boolean {
    return this._available
  }

  async init(): Promise<void> {
    try {
      ort.env.logLevel = 'error'
      ort.env.wasm.wasmPaths = ORT_BASE

      // 并行加载 det + rec
      await Promise.all([
        faceDetector.init().then(() => {
          this._detAvailable = faceDetector.available
        }),
        (async () => {
          const res = await fetch(REC_MODEL_URL)
          if (!res.ok) throw new Error(`rec model fetch ${res.status}`)
          const buf = await res.arrayBuffer()
          this.recSession = await ort.InferenceSession.create(buf, {
            executionProviders: ['wasm'],
          })
          this.recInputName = this.recSession.inputNames[0] ?? 'input.1'
          this.recOutputName = this.recSession.outputNames[0] ?? '683'
        })(),
      ])
      this._available = true
    } catch (e) {
      console.warn('[FaceVerifier] init failed (fail-open)', e)
      this._available = false
    }
  }

  /** 中心 square crop（det 模型不可用时的 fallback） */
  private centerFaceCrop(img: ImageData): { x: number; y: number; size: number } {
    const side = Math.floor(Math.min(img.width, img.height) * 0.82)
    const x = Math.floor((img.width - side) / 2)
    const y = Math.floor((img.height - side) / 2)
    return { x, y, size: side }
  }

  /** 解析 crop 区域：优先 SCRFD；det 可用但未检出脸则返回 null */
  private async resolveCrop(img: ImageData): Promise<{ rect: FaceCropRect; fromDet: boolean } | null> {
    if (this._detAvailable) {
      const detCrop = await faceDetector.detectBestCrop(img)
      if (detCrop && detCrop.size >= MIN_FACE_SIDE) {
        return { rect: detCrop, fromDet: true }
      }
      return null
    }
    return { rect: this.centerFaceCrop(img), fromDet: false }
  }

  async extractFromJPEG(
    jpeg: ArrayBuffer,
  ): Promise<{ emb: Float32Array; detected: boolean; fromDet: boolean } | null> {
    if (!this._available || !this.recSession) return null
    const img = await jpegToImageData(jpeg)
    if (!img) return null

    const crop = await this.resolveCrop(img)
    if (!crop) return null

    const { rect } = crop
    const tensorData = cropAndNormalize(img, rect.x, rect.y, rect.size)
    if (!tensorData) return null

    const input = new ort.Tensor('float32', tensorData, [1, 3, INPUT_SIZE, INPUT_SIZE])
    const feeds: Record<string, ort.Tensor> = { [this.recInputName]: input }
    const out = await this.recSession.run(feeds)
    const tensor = out[this.recOutputName]
    if (!tensor?.data) return null

    const raw =
      tensor.data instanceof Float32Array
        ? tensor.data
        : new Float32Array(tensor.data as ArrayLike<number>)
    const emb = FaceVerifier.l2Normalize(new Float32Array(raw))
    return { emb, detected: true, fromDet: crop.fromDet }
  }

  async verify(
    jpeg: ArrayBuffer,
    ownerEmbedding: Float32Array,
    threshold = 0.42,
  ): Promise<FaceVerifyResult | null> {
    const extracted = await this.extractFromJPEG(jpeg)
    if (!extracted) return null
    const score = FaceVerifier.cosineSimilarity(extracted.emb, ownerEmbedding)
    // 低分多为 crop 未对准脸，不应上报 detected=true 导致服务端误判 unknown
    const detected = score >= MIN_FACE_DETECT_SCORE
    return {
      match: detected && score >= threshold,
      score,
      detected,
    }
  }

  /** 多张 enrollment 帧 → 平均 embedding */
  async enrollFromFrames(jpegs: ArrayBuffer[]): Promise<Float32Array | null> {
    const embs: Float32Array[] = []
    for (const jpeg of jpegs) {
      const ex = await this.extractFromJPEG(jpeg)
      if (ex?.emb) embs.push(ex.emb)
    }
    if (embs.length === 0) return null
    return FaceVerifier.averageEmbeddings(embs)
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
    return FaceVerifier.l2Normalize(sum)
  }

  destroy() {
    this.recSession?.release()
    this.recSession = null
    this._available = false
    this._detAvailable = false
    faceDetector.destroy()
  }
}
