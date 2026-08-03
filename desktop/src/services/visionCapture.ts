/**
 * Mochi 视觉感知（客户端）
 *
 * 架构：前置摄像头是 Mochi 获取外界画面信息的**唯一来源**；
 * 会话级 MediaStream + turn 内 JPEG 快拍，供服务端 VL 理解表情/举物/场景。
 *
 * 产品表达：对主人不说「摄像头/拍到」，Mochi 以「眼睛、看清楚」等人格口吻说话
 * （见 server/internal/vision/prompts.go、prompt/layers.go 规则 16）。
 */

import { getClientConfig } from '@/config'
import { getVisionCaptureMinIntervalMs } from '@/services/lowPowerMode'
import { isTauri } from '@/services/chatWindow'

const VISION_JPEG_QUALITY = 0.85
const WARMUP_MS = 180

/** 上次成功抓拍时间戳，低配 5fps 节流用。 */
let lastSnapshotAtMs = 0

export type VisionFrameReason = 'speech_start' | 'audio_end' | 'object_refresh' | 'pause_probe' | 'glance'

export type VisionStartResult = 'ok' | 'denied' | 'unavailable' | 'skipped'

/** 客户端是否开启视觉抓拍（localStorage 显式关=关；未设置且服务端开 vision=默认开）。 */
export function isVisionCaptureEnabled(): boolean {
  const stored = localStorage.getItem('mochi_vision_enabled')
  if (stored === '0') return false
  if (stored === '1') return true
  return getClientConfig().visionEnabled
}

export function setVisionCaptureEnabled(on: boolean) {
  localStorage.setItem('mochi_vision_enabled', on ? '1' : '0')
}

/** 服务端开启 vision 时，将未设置过的 localStorage 默认设为开。 */
export function ensureVisionCaptureDefault() {
  if (!getClientConfig().visionEnabled) return
  if (localStorage.getItem('mochi_vision_enabled') == null) {
    localStorage.setItem('mochi_vision_enabled', '1')
  }
}

/** 是否应使用会话级摄像头（服务端 + 客户端开关 + session_camera 配置）。 */
export function isVisionSessionWanted(): boolean {
  if (!getClientConfig().visionEnabled) return false
  if (!isVisionCaptureEnabled()) return false
  return getClientConfig().visionSessionCamera !== false
}

/** 摄像头被拒绝时的提示（Tauri WebView2 vs 浏览器）。 */
export function cameraPermissionDeniedMessage(): string {
  if (isTauri()) {
    return (
      '摄像头被拒绝，无法「察言观色」。请打开 Windows 设置 → 隐私 → 摄像头，' +
      '开启「桌面应用」访问；并在设置里打开「语音时看我」。'
    )
  }
  return '摄像头被拒绝。请在浏览器网站设置中允许摄像头，并在 Mochi 设置里打开「语音时看我」。'
}

/** 等待 video 下一帧就绪（离屏 video 也需 tick 才有最新画面）。 */
async function waitForVideoFrame(video: HTMLVideoElement): Promise<void> {
  const rvfc = (video as HTMLVideoElement & {
    requestVideoFrameCallback?: (cb: () => void) => number
  }).requestVideoFrameCallback
  if (typeof rvfc === 'function') {
    await new Promise<void>((resolve) => {
      rvfc.call(video, () => resolve())
    })
    return
  }
  await new Promise((r) => setTimeout(r, 66))
}

/** 从 video 元素绘制 JPEG（镜像前置摄像头，便于举物对准画面）。 */
async function snapshotFromVideo(video: HTMLVideoElement): Promise<ArrayBuffer | null> {
  await waitForVideoFrame(video)
  const w = video.videoWidth
  const h = video.videoHeight
  if (!w || !h) {
    console.warn('[vision] video not ready %dx%d', w, h)
    return null
  }
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    console.warn('[vision] canvas 2d unavailable')
    return null
  }
  // 镜像自拍预览，与用户举物直觉一致
  ctx.translate(w, 0)
  ctx.scale(-1, 1)
  ctx.drawImage(video, 0, 0, w, h)
  const blob = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/jpeg', VISION_JPEG_QUALITY)
  })
  if (!blob || blob.size === 0) {
    console.warn('[vision] empty jpeg blob')
    return null
  }
  return await blob.arrayBuffer()
}

/**
 * 会话级摄像头：startTalk 后保持 MediaStream，turn 内 grabSnapshot 快拍，endConversation 关闭。
 */
export class VisionSession {
  private stream: MediaStream | null = null
  private video: HTMLVideoElement | null = null
  private warming: Promise<void> | null = null
  private active = false

  isActive(): boolean {
    return this.active && !!this.stream && !!this.video
  }

  /** 打开前置摄像头并 warm up；已 active 时幂等返回 ok。 */
  async start(): Promise<VisionStartResult> {
    if (!isVisionSessionWanted()) return 'skipped'
    if (this.active) return 'ok'
    if (!navigator.mediaDevices?.getUserMedia) {
      console.warn('[vision] getUserMedia unavailable')
      return 'unavailable'
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: 'user',
          width: { ideal: 640 },
          height: { ideal: 480 },
        },
        audio: false,
      })
      const video = document.createElement('video')
      video.srcObject = stream
      video.muted = true
      video.playsInline = true
      video.setAttribute('aria-hidden', 'true')
      Object.assign(video.style, {
        position: 'fixed',
        width: '1px',
        height: '1px',
        opacity: '0',
        pointerEvents: 'none',
        left: '-9999px',
        top: '0',
      })
      document.body.appendChild(video)

      this.stream = stream
      this.video = video
      this.active = true
      this.warming = this.warmVideo(video)
      await this.warming
      console.info('[vision] session_started')
      return 'ok'
    } catch (e) {
      const name = e instanceof DOMException ? e.name : 'unknown'
      console.warn('[vision] session_start failed name=%s', name, e)
      await this.stop()
      if (name === 'NotAllowedError' || name === 'PermissionDeniedError') {
        return 'denied'
      }
      return 'unavailable'
    }
  }

  private async warmVideo(video: HTMLVideoElement): Promise<void> {
    await video.play()
    await new Promise<void>((resolve) => {
      if (video.videoWidth > 0 && video.videoHeight > 0) {
        resolve()
        return
      }
      const onReady = () => {
        if (video.videoWidth > 0 && video.videoHeight > 0) {
          video.removeEventListener('loadeddata', onReady)
          resolve()
        }
      }
      video.addEventListener('loadeddata', onReady)
      setTimeout(resolve, WARMUP_MS)
    })
    await new Promise((r) => setTimeout(r, WARMUP_MS))
  }

  /** 从已 warm 的流抓拍 JPEG；未 start 返回 null。 */
  async grabSnapshot(): Promise<ArrayBuffer | null> {
    if (!this.isActive() || !this.video) return null
    const minInterval = getVisionCaptureMinIntervalMs()
    if (minInterval > 0) {
      const elapsed = Date.now() - lastSnapshotAtMs
      if (lastSnapshotAtMs > 0 && elapsed < minInterval) {
        await new Promise((r) => setTimeout(r, minInterval - elapsed))
      }
    }
    if (this.warming) {
      await this.warming.catch(() => {})
    }
    const buf = await snapshotFromVideo(this.video)
    if (buf) {
      lastSnapshotAtMs = Date.now()
      console.info('[vision] snapshot_ok bytes=%d', buf.byteLength)
    }
    return buf
  }

  /** 关闭摄像头并释放资源；幂等。 */
  async stop(): Promise<void> {
    if (!this.active && !this.stream && !this.video) return
    this.active = false
    this.warming = null
    this.stream?.getTracks().forEach((t) => t.stop())
    this.stream = null
    if (this.video) {
      this.video.srcObject = null
      this.video.remove()
      this.video = null
    }
    console.info('[vision] session_stopped')
  }
}

/** 全局单例：整段对话共用一条 camera 流。 */
export const visionSession = new VisionSession()

/** 启动会话级摄像头（startTalk 后调用）。 */
export async function startVisionSession(): Promise<VisionStartResult> {
  return visionSession.start()
}

/**
 * Phase D #12：后台预热摄像头 session（登录/ambient 或 startTalk 并行调用）。
 * 幂等；失败不抛错。
 */
export function prewarmVisionSession(): Promise<VisionStartResult> {
  if (!isVisionSessionWanted()) return Promise.resolve('skipped')
  return startVisionSession().catch(() => 'unavailable' as VisionStartResult)
}

/** 结束会话级摄像头（endConversation / pauseVoiceForText 调用）。 */
export async function stopVisionSession(): Promise<void> {
  await visionSession.stop()
}

/**
 * 抓拍前置摄像头一帧 JPEG（冷启动 fallback；会话流未开时使用）。
 * 失败返回 null，不抛错，便于语音链路继续。
 */
export async function captureOwnerFaceJPEG(): Promise<ArrayBuffer | null> {
  if (visionSession.isActive()) {
    return visionSession.grabSnapshot()
  }
  if (!navigator.mediaDevices?.getUserMedia) {
    console.warn('[vision] getUserMedia unavailable')
    return null
  }
  let stream: MediaStream | null = null
  let video: HTMLVideoElement | null = null
  try {
    stream = await navigator.mediaDevices.getUserMedia({
      video: {
        facingMode: 'user',
        width: { ideal: 640 },
        height: { ideal: 480 },
      },
      audio: false,
    })
    video = document.createElement('video')
    video.srcObject = stream
    video.muted = true
    video.playsInline = true
    await video.play()
    await new Promise((r) => setTimeout(r, WARMUP_MS))
    const buf = await snapshotFromVideo(video)
    if (buf) {
      console.info('[vision] capture_ok bytes=%d (cold)', buf.byteLength)
    }
    return buf
  } catch (e) {
    const name = e instanceof DOMException ? e.name : 'unknown'
    console.warn('[vision] capture failed name=%s', name, e)
    return null
  } finally {
    stream?.getTracks().forEach((t) => t.stop())
    if (video) {
      video.srcObject = null
    }
  }
}

/** ArrayBuffer → base64（不含 data URL 前缀）。 */
export function jpegToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}
