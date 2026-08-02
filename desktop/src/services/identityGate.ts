/**
 * 视听融合门控（P2）：声纹主门控 + 人脸灰区加分。
 *
 * 场景① 非主人直接对 Mochi 说话 → non_owner TTS（仅声纹触发，人脸不单独拒答）
 * 场景② 主人会话中背景他人声 → 静默过滤 PCM
 * 声纹灰区 + 主人脸可见 → owner_boost 倾向放行
 */

import type { RealtimeFaceprintConfig, RealtimeVoiceprintConfig } from '@/config'
import type { SpeakerVerifyResult } from '@/services/speakerGate'
import { FACE_OWNER_BOOST_SCORE } from '@/services/faceVerifier'

export interface FaceVerifyResult {
  match: boolean
  score: number
  detected: boolean
}

export type IdentityMode =
  | 'owner'
  | 'owner_boost'
  | 'filter_foreign'
  | 'foreign_only'
  | 'open'

export class IdentityGate {
  private lastOwnerMatchAt = 0
  private rejectStreak = 0
  private uploadAllowed = true
  private lastNonOwnerReplyAt = 0
  /** 最近一次有效人脸 probe */
  lastFaceResult: FaceVerifyResult | null = null

  resetTurn() {
    this.rejectStreak = 0
    this.uploadAllowed = true
    this.lastFaceResult = null
  }

  markOwnerMatch() {
    this.lastOwnerMatchAt = Date.now()
    this.rejectStreak = 0
    this.uploadAllowed = true
  }

  /** 缓存人脸 probe，供 stream_check 融合。 */
  noteFaceResult(face: FaceVerifyResult | null) {
    if (face?.detected) {
      this.lastFaceResult = face
    }
  }

  /** 声纹是否在灰区（接近阈值但未达 match） */
  private isVoiceGrayZone(score: number, voiceCfg: RealtimeVoiceprintConfig, faceCfg: RealtimeFaceprintConfig): boolean {
    const low = faceCfg.grayZoneLow ?? 0.28
    return score >= low && score < voiceCfg.threshold
  }

  /**
   * 融合声纹 + 人脸更新 upload 门控。
   */
  applyIdentityResult(
    voice: SpeakerVerifyResult | null,
    face: FaceVerifyResult | null,
    voiceCfg: RealtimeVoiceprintConfig,
    faceCfg: RealtimeFaceprintConfig,
  ): IdentityMode {
    if (face?.detected) {
      this.lastFaceResult = face
    }

    const voiceOpen = !voiceCfg.required || voice == null
    if (voiceOpen) {
      this.uploadAllowed = true
      return 'open'
    }

    if (voice.match) {
      this.markOwnerMatch()
      return 'owner'
    }

    // P2：声纹灰区 + 主人脸（match 或高分）→ 倾向放行（降误拒）
    if (
      faceCfg.enabled &&
      face &&
      (face.match || face.score >= FACE_OWNER_BOOST_SCORE) &&
      this.isVoiceGrayZone(voice.score, voiceCfg, faceCfg)
    ) {
      this.markOwnerMatch()
      return 'owner_boost'
    }

    const ownerRecentMs = voiceCfg.ownerRecentMs ?? faceCfg.ownerRecentMs ?? 8000
    const ownerStillActive = Date.now() - this.lastOwnerMatchAt < ownerRecentMs

    // 主人会话窗口内 + 人脸已确认：声纹瞬时偏低也不断 PCM（避免 ASR 空、识别不到）
    if (
      ownerStillActive &&
      faceCfg.enabled &&
      face?.detected &&
      (face.match || face.score >= FACE_OWNER_BOOST_SCORE)
    ) {
      this.uploadAllowed = true
      return 'owner_boost'
    }

    this.rejectStreak++

    if (ownerStillActive) {
      this.uploadAllowed = false
      return 'filter_foreign'
    }

    this.uploadAllowed = false
    return 'foreign_only'
  }

  /** 兼容 P0 单声纹路径 */
  applyVerifyResult(result: SpeakerVerifyResult | null, cfg: RealtimeVoiceprintConfig): IdentityMode {
    return this.applyIdentityResult(result, this.lastFaceResult, cfg, { enabled: false } as RealtimeFaceprintConfig)
  }

  shouldAllowUpload(): boolean {
    return this.uploadAllowed
  }

  shouldTriggerNonOwnerReply(cfg: RealtimeVoiceprintConfig, opts?: { immediate?: boolean }): boolean {
    if (!cfg.required) return false
    const cooldown = cfg.nonOwnerReplyCooldownMs ?? 12000
    if (Date.now() - this.lastNonOwnerReplyAt < cooldown) return false
    if (opts?.immediate) return true
    const streakNeed = cfg.rejectStreak ?? 3
    const ownerRecentMs = cfg.ownerRecentMs ?? 8000
    const noRecentOwner = Date.now() - this.lastOwnerMatchAt >= ownerRecentMs
    return noRecentOwner && this.rejectStreak >= streakNeed
  }

  markNonOwnerReplySent() {
    this.lastNonOwnerReplyAt = Date.now()
    this.rejectStreak = 0
  }
}

export const identityGate = new IdentityGate()
