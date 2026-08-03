/**
 * Phase E — 眼（EyeChannel）：视觉帧策略、capture、face probe、submit 前 Barrier。
 */

import { getClientConfig, getFaceprintConfig } from '@/config'
import { shouldThrottleVisionCapture } from '@/services/lowPowerMode'
import {
  captureOwnerFaceJPEG,
  isVisionCaptureEnabled,
  jpegToBase64,
  visionSession,
  type VisionFrameReason,
} from '@/services/visionCapture'
import { looksLikeObjectQuery, needsHardFocusFrame } from '@/services/visionHeuristic'
import {
  HARD_FOCUS_SEND_MS,
  perceptionAllowsVision,
  type FaceProbe,
  type PerceptionPhase,
} from './types'

export interface EyeSendMeta {
  reason: VisionFrameReason
  faceProbe?: FaceProbe
  partialText?: string
}

export interface EyeChannelDeps {
  getPhase: () => PerceptionPhase
  getAbortSignal: () => AbortSignal | undefined
  sendVisionFrame: (b64: string, meta: EyeSendMeta) => boolean
  probeFaceFromJPEG: (buf: ArrayBuffer) => Promise<FaceProbe | null>
  faceprintReady: () => boolean
  onSkip?: (reason: string, trigger: VisionFrameReason) => void
}

/** 配置 + 相位 + 低配节流：是否应发送该 reason 的帧。 */
export function shouldSendVisionFrame(
  reason: VisionFrameReason,
  phase: PerceptionPhase,
): { ok: boolean; skipReason?: string } {
  if (!getClientConfig().visionEnabled) {
    return { ok: false, skipReason: 'server_disabled' }
  }
  if (!isVisionCaptureEnabled()) {
    return { ok: false, skipReason: 'client_opt_out' }
  }
  if (!perceptionAllowsVision(reason, phase)) {
    return { ok: false, skipReason: 'phase_blocked' }
  }
  const cfg = getClientConfig()
  if (reason === 'speech_start' && !cfg.visionSnapshotOnSpeechStart) {
    return { ok: false, skipReason: 'config_speech_start_off' }
  }
  if (reason === 'audio_end' && cfg.visionSnapshotOnAudioEnd === false) {
    return { ok: false, skipReason: 'config_audio_end_off' }
  }
  if (reason === 'object_refresh' && cfg.visionSnapshotOnObjectIntent === false) {
    return { ok: false, skipReason: 'config_object_intent_off' }
  }
  if (shouldThrottleVisionCapture(reason)) {
    return { ok: false, skipReason: 'low_power_throttle' }
  }
  return { ok: true }
}

/** 是否应对该帧做 Tier-0 faceDet probe。 */
export function shouldProbeFaceOnFrame(reason: VisionFrameReason): boolean {
  const fp = getFaceprintConfig()
  if (!fp.enabled) return false
  if (reason === 'pause_probe' || reason === 'glance') return true
  if (reason !== 'speech_start') return true
  return !!fp.probeOnSpeechStart
}

export class EyeChannel {
  constructor(private deps: EyeChannelDeps) {}

  /** 抓拍 JPEG；失败返回 null。 */
  async captureJPEG(): Promise<ArrayBuffer | null> {
    if (this.deps.getAbortSignal()?.aborted) return null
    return visionSession.isActive()
      ? visionSession.grabSnapshot()
      : captureOwnerFaceJPEG()
  }

  /** 发送单帧视觉数据（含可选 face probe）。 */
  async sendFrame(reason: VisionFrameReason, asrPartial = ''): Promise<boolean> {
    const phase = this.deps.getPhase()
    const gate = shouldSendVisionFrame(reason, phase)
    if (!gate.ok) {
      this.deps.onSkip?.(gate.skipReason ?? 'unknown', reason)
      return false
    }

    const start = Date.now()
    if (this.deps.getAbortSignal()?.aborted) return false

    const buf = await this.captureJPEG()
    if (!buf) {
      if (import.meta.env.DEV) {
        console.warn(
          '[vision] skip_send reason=capture_failed trigger=%s elapsed_ms=%d',
          reason,
          Date.now() - start,
        )
      }
      return false
    }

    const b64 = jpegToBase64(buf)
    let faceProbe: FaceProbe | undefined
    if (shouldProbeFaceOnFrame(reason) && this.deps.faceprintReady()) {
      const face = await this.deps.probeFaceFromJPEG(buf)
      if (face) {
        faceProbe = face
        if (import.meta.env.DEV) {
          console.debug(
            '[faceprint] frame_probe reason=%s match=%s score=%s',
            reason,
            face.match,
            face.score.toFixed(3),
          )
        }
      }
    }

    const ok = this.deps.sendVisionFrame(b64, {
      reason,
      faceProbe,
      partialText: reason === 'pause_probe' ? asrPartial : undefined,
    })
    console.info(
      '[vision] frame_sent ok=%s trigger=%s jpeg_bytes=%d b64_len=%d elapsed_ms=%d',
      ok,
      reason,
      buf.byteLength,
      b64.length,
      Date.now() - start,
    )
    return ok
  }

  /**
   * Phase B/C：owner_face 异步；硬焦点 150ms 发帧窗口后再 submit。
   */
  async prepareBeforeSubmit(turnText: string): Promise<void> {
    const hardFocus = needsHardFocusFrame(turnText)
    if (hardFocus) {
      await Promise.race([
        (async () => {
          if (
            getClientConfig().visionSnapshotOnObjectIntent &&
            looksLikeObjectQuery(turnText)
          ) {
            await this.sendFrame('object_refresh')
          }
          await this.sendFrame('audio_end')
        })(),
        new Promise<void>((r) => setTimeout(r, HARD_FOCUS_SEND_MS)),
      ])
    } else {
      void this.sendFrame('audio_end')
    }
  }
}
