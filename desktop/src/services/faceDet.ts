/**
 * SCRFD 人脸检测（InsightFace det_10g.onnx，浏览器 WASM）。
 * 参考 insightface/model_zoo/scrfd.py
 */
import * as ort from 'onnxruntime-web/wasm'

const ORT_BASE = 'https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/'
const DET_MODEL_URL = '/models/face/det.onnx'
const DET_SIZE = 640
const DET_THRESH = 0.5
const NMS_THRESH = 0.4

export interface FaceBox {
  x1: number
  y1: number
  x2: number
  y2: number
  score: number
}

/** 用于 rec 模型的 square crop 区域（原图像素坐标） */
export interface FaceCropRect {
  x: number
  y: number
  size: number
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v))
}

function distance2bbox(
  points: Float32Array,
  distance: Float32Array,
  numPoints: number,
): Float32Array {
  const bboxes = new Float32Array(numPoints * 4)
  for (let i = 0; i < numPoints; i++) {
    const px = points[i * 2]
    const py = points[i * 2 + 1]
    bboxes[i * 4] = px - distance[i * 4]
    bboxes[i * 4 + 1] = py - distance[i * 4 + 1]
    bboxes[i * 4 + 2] = px + distance[i * 4 + 2]
    bboxes[i * 4 + 3] = py + distance[i * 4 + 3]
  }
  return bboxes
}

function nms(dets: FaceBox[], thresh: number): FaceBox[] {
  if (dets.length === 0) return []
  const sorted = [...dets].sort((a, b) => b.score - a.score)
  const keep: FaceBox[] = []
  const suppressed = new Set<number>()

  for (let i = 0; i < sorted.length; i++) {
    if (suppressed.has(i)) continue
    const a = sorted[i]
    keep.push(a)
    for (let j = i + 1; j < sorted.length; j++) {
      if (suppressed.has(j)) continue
      const b = sorted[j]
      const xx1 = Math.max(a.x1, b.x1)
      const yy1 = Math.max(a.y1, b.y1)
      const xx2 = Math.min(a.x2, b.x2)
      const yy2 = Math.min(a.y2, b.y2)
      const w = Math.max(0, xx2 - xx1 + 1)
      const h = Math.max(0, yy2 - yy1 + 1)
      const inter = w * h
      const areaA = (a.x2 - a.x1 + 1) * (a.y2 - a.y1 + 1)
      const areaB = (b.x2 - b.x1 + 1) * (b.y2 - b.y1 + 1)
      const ovr = inter / (areaA + areaB - inter)
      if (ovr > thresh) suppressed.add(j)
    }
  }
  return keep
}

/** bbox → 带边距的 square crop（InsightFace 风格） */
export function faceBoxToCrop(box: FaceBox, imgW: number, imgH: number, pad = 1.15): FaceCropRect {
  const w = box.x2 - box.x1
  const h = box.y2 - box.y1
  const cx = (box.x1 + box.x2) / 2
  const cy = (box.y1 + box.y2) / 2
  let side = Math.max(w, h) * pad
  let x = cx - side / 2
  let y = cy - side / 2
  // 限制在画面内
  if (x < 0) x = 0
  if (y < 0) y = 0
  if (x + side > imgW) side = imgW - x
  if (y + side > imgH) side = imgH - y
  return { x: Math.floor(x), y: Math.floor(y), size: Math.floor(side) }
}

export class FaceDetector {
  private session: ort.InferenceSession | null = null
  private inputName = 'input.1'
  private outputNames: string[] = []
  private fmc = 3
  private featStrides = [8, 16, 32]
  private numAnchors = 2
  private centerCache = new Map<string, Float32Array>()
  _available = false

  get available(): boolean {
    return this._available
  }

  async init(): Promise<void> {
    try {
      ort.env.logLevel = 'error'
      ort.env.wasm.wasmPaths = ORT_BASE
      const res = await fetch(DET_MODEL_URL)
      if (!res.ok) {
        console.info('[FaceDetector] det.onnx not found, using center crop fallback')
        return
      }
      const buf = await res.arrayBuffer()
      this.session = await ort.InferenceSession.create(buf, {
        executionProviders: ['wasm'],
      })
      this.inputName = this.session.inputNames[0] ?? 'input.1'
      this.outputNames = [...this.session.outputNames]
      // buffalo_l det_10g：9 路输出 = 3×score + 3×bbox + 3×kps（stride 8/16/32）
      const outCount = this.outputNames.length
      if (outCount === 9 || outCount === 6) {
        this.fmc = 3
        this.featStrides = [8, 16, 32]
        this.numAnchors = 2
      }
      this._available = true
      console.info('[FaceDetector] det.onnx loaded outputs=%d names=%o', outCount, this.outputNames)
    } catch (e) {
      console.warn('[FaceDetector] init failed (center crop fallback)', e)
      this._available = false
    }
  }

  /** letterbox 缩放到 DET_SIZE，返回 blob [1,3,H,W] 与缩放比 */
  private prepareBlob(img: ImageData): { blob: Float32Array; scale: number; padW: number; padH: number } {
    const { width: iw, height: ih } = img
    const imRatio = ih / iw
    const modelRatio = 1 // 640x640 square
    let newW: number
    let newH: number
    if (imRatio > modelRatio) {
      newH = DET_SIZE
      newW = Math.floor(newH / imRatio)
    } else {
      newW = DET_SIZE
      newH = Math.floor(newW * imRatio)
    }
    const scale = newH / ih

    const canvas = document.createElement('canvas')
    canvas.width = DET_SIZE
    canvas.height = DET_SIZE
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('canvas ctx')

    const tmp = document.createElement('canvas')
    tmp.width = iw
    tmp.height = ih
    const tctx = tmp.getContext('2d')
    if (!tctx) throw new Error('tmp ctx')
    tctx.putImageData(img, 0, 0)
    ctx.fillStyle = '#000'
    ctx.fillRect(0, 0, DET_SIZE, DET_SIZE)
    ctx.drawImage(tmp, 0, 0, newW, newH)

    const resized = ctx.getImageData(0, 0, DET_SIZE, DET_SIZE)
    const blob = new Float32Array(3 * DET_SIZE * DET_SIZE)
    const rd = resized.data
    // RGB，swapRB 与 OpenCV blobFromImage(swapRB=true) 一致 → 实际输入 BGR 顺序在 ORT 侧按 RGB 训练
    // det_10g 训练用 BGR；canvas 为 RGBA，按 R,G,B 填入 CHW，与 insightface cv2 读入 BGR 后 swapRB 等效
    let o = 0
    for (let c = 0; c < 3; c++) {
      for (let i = c; i < rd.length; i += 4) {
        blob[o++] = (rd[i] - 127.5) / 128
      }
    }
    return { blob, scale, padW: 0, padH: 0 }
  }

  private getAnchorCenters(height: number, width: number, stride: number): Float32Array {
    const key = `${height}x${width}x${stride}`
    const cached = this.centerCache.get(key)
    if (cached) return cached

    const centers: number[] = []
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        for (let a = 0; a < this.numAnchors; a++) {
          centers.push(x * stride, y * stride)
        }
      }
    }
    const out = new Float32Array(centers)
    if (this.centerCache.size < 100) this.centerCache.set(key, out)
    return out
  }

  private tensorToFlat(t: ort.Tensor): Float32Array {
    if (t.data instanceof Float32Array) return t.data
    return new Float32Array(t.data as ArrayLike<number>)
  }

  /** 检测所有人脸，坐标映射回原图 */
  async detect(img: ImageData, maxNum = 1): Promise<FaceBox[]> {
    if (!this.session || !this._available) return []

    const { blob, scale } = this.prepareBlob(img)
    const input = new ort.Tensor('float32', blob, [1, 3, DET_SIZE, DET_SIZE])
    const outMap = await this.session.run({ [this.inputName]: input })

    const candidates: FaceBox[] = []
    const inputHeight = DET_SIZE
    const inputWidth = DET_SIZE

    for (let idx = 0; idx < this.featStrides.length; idx++) {
      const stride = this.featStrides[idx]
      const scoreName = this.outputNames[idx]
      const bboxName = this.outputNames[idx + this.fmc]
      if (!scoreName || !bboxName) continue

      let scores = this.tensorToFlat(outMap[scoreName])
      let bboxPreds = this.tensorToFlat(outMap[bboxName])

      const numPoints = bboxPreds.length / 4
      if (scores.length > numPoints) {
        // [N,1] → 取前 N 个
        scores = scores.subarray(0, numPoints)
      }
      for (let i = 0; i < numPoints; i++) {
        bboxPreds[i * 4] *= stride
        bboxPreds[i * 4 + 1] *= stride
        bboxPreds[i * 4 + 2] *= stride
        bboxPreds[i * 4 + 3] *= stride
      }

      const height = inputHeight / stride
      const width = inputWidth / stride
      const anchorCenters = this.getAnchorCenters(height, width, stride)
      const bboxes = distance2bbox(anchorCenters, bboxPreds, numPoints)

      for (let i = 0; i < numPoints; i++) {
        const score = scores[i]
        if (score < DET_THRESH) continue
        const x1 = bboxes[i * 4] / scale
        const y1 = bboxes[i * 4 + 1] / scale
        const x2 = bboxes[i * 4 + 2] / scale
        const y2 = bboxes[i * 4 + 3] / scale
        candidates.push({
          x1: clamp(x1, 0, img.width),
          y1: clamp(y1, 0, img.height),
          x2: clamp(x2, 0, img.width),
          y2: clamp(y2, 0, img.height),
          score,
        })
      }
    }

    const kept = nms(candidates, NMS_THRESH)
    if (maxNum > 0) return kept.slice(0, maxNum)
    return kept
  }

  /** 取面积最大的人脸 crop */
  async detectBestCrop(img: ImageData): Promise<FaceCropRect | null> {
    const boxes = await this.detect(img, 5)
    if (boxes.length === 0) return null
    boxes.sort((a, b) => {
      const areaA = (a.x2 - a.x1) * (a.y2 - a.y1)
      const areaB = (b.x2 - b.x1) * (b.y2 - b.y1)
      return areaB - areaA
    })
    return faceBoxToCrop(boxes[0], img.width, img.height)
  }

  destroy() {
    this.session?.release()
    this.session = null
    this._available = false
    this.centerCache.clear()
  }
}

export const faceDetector = new FaceDetector()
