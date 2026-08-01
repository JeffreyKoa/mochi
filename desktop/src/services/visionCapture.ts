/**
 * 语音 turn 前抓拍主人面部 JPEG（V1：单次单帧，用完即关摄像头）。
 */

const VISION_JPEG_QUALITY = 0.85
const WARMUP_MS = 180

/** 客户端是否已 opt-in 视觉（localStorage）。 */
export function isVisionCaptureEnabled(): boolean {
  return localStorage.getItem('mochi_vision_enabled') === '1'
}

export function setVisionCaptureEnabled(on: boolean) {
  localStorage.setItem('mochi_vision_enabled', on ? '1' : '0')
}

/** 抓拍前置摄像头一帧 JPEG；失败返回 null（不抛错，便于语音链路继续）。 */
export async function captureOwnerFaceJPEG(): Promise<ArrayBuffer | null> {
  if (!navigator.mediaDevices?.getUserMedia) {
    console.warn('[vision] getUserMedia unavailable')
    return null
  }
  let stream: MediaStream | null = null
  try {
    stream = await navigator.mediaDevices.getUserMedia({
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
    await video.play()
    await new Promise((r) => setTimeout(r, WARMUP_MS))

    const w = video.videoWidth || 640
    const h = video.videoHeight || 480
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      console.warn('[vision] canvas 2d unavailable')
      return null
    }
    ctx.drawImage(video, 0, 0, w, h)
    const blob = await new Promise<Blob | null>((resolve) => {
      canvas.toBlob(resolve, 'image/jpeg', VISION_JPEG_QUALITY)
    })
    if (!blob || blob.size === 0) {
      console.warn('[vision] empty jpeg blob')
      return null
    }
    console.info('[vision] capture_ok bytes=%d %dx%d', blob.size, w, h)
    return await blob.arrayBuffer()
  } catch (e) {
    console.warn('[vision] capture failed', e)
    return null
  } finally {
    stream?.getTracks().forEach((t) => t.stop())
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
