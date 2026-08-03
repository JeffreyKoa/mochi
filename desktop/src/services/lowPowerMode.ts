/**
 * Phase D 低配模式：降 PIXI 帧率、限视觉抓拍 5fps、关周期 ONNX 验脸。
 * 开关来自服务端 public config `client.low_power_mode`。
 */

import { getClientConfig } from '@/config'
import type { VisionFrameReason } from '@/services/visionCapture'

/** 正常模式宠物动画 tick（~20fps）。 */
const PET_ANIM_NORMAL_MS = 50
/** 低配模式宠物动画 tick（~15fps，低于方案 30fps 上限以进一步省 CPU）。 */
const PET_ANIM_LOW_POWER_MS = 66
/** 低配模式视觉抓拍最小间隔（5fps）。 */
const VISION_CAPTURE_MIN_MS = 200

export function isLowPowerMode(): boolean {
  return getClientConfig().lowPowerMode
}

/** 宠物 PIXI 动画 setInterval 间隔（毫秒）。 */
export function getPetAnimIntervalMs(): number {
  return isLowPowerMode() ? PET_ANIM_LOW_POWER_MS : PET_ANIM_NORMAL_MS
}

/** 是否启用周期性 ONNX 验脸（低配关，speech_start / pause_probe 仍走帧内 probe）。 */
export function shouldRunPeriodicFaceCheck(): boolean {
  return !isLowPowerMode()
}

/** 视觉抓拍最小间隔；正常模式为 0（不节流）。 */
export function getVisionCaptureMinIntervalMs(): number {
  return isLowPowerMode() ? VISION_CAPTURE_MIN_MS : 0
}

/**
 * 低配下是否应对该 reason 做 5fps 节流。
 * audio_end / object_refresh 为 Barrier 硬依赖，不节流。
 */
export function shouldThrottleVisionCapture(reason: VisionFrameReason): boolean {
  if (!isLowPowerMode()) return false
  return reason !== 'audio_end' && reason !== 'object_refresh'
}
