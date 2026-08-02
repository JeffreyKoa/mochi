/**
 * 声纹门控（P0）：对话中持续验主人声纹。
 *
 * 场景① 非主人直接对 Mochi 说话 → 由调用方触发 non_owner_turn 口语拒答。
 * 场景② 主人正在说话、背景有他人声 → uploadAllowed=false，静默过滤 PCM。
 */

import type { RealtimeVoiceprintConfig } from '@/config'

export interface SpeakerVerifyResult {
  match: boolean
  score: number
}

export class SpeakerGate {
  private lastOwnerMatchAt = 0
  private rejectStreak = 0
  private uploadAllowed = true
  private lastNonOwnerReplyAt = 0

  /** 重置 turn / 进入 resting 时调用。 */
  resetTurn() {
    this.rejectStreak = 0
    this.uploadAllowed = true
  }

  /** 唤醒成功或 stream_check 匹配主人。 */
  markOwnerMatch() {
    this.lastOwnerMatchAt = Date.now()
    this.rejectStreak = 0
    this.uploadAllowed = true
  }

  /**
   * 根据最近一次验声结果更新门控。
   * @returns mode 供日志：owner | filter_foreign | foreign_only
   */
  applyVerifyResult(result: SpeakerVerifyResult | null, cfg: RealtimeVoiceprintConfig): 'owner' | 'filter_foreign' | 'foreign_only' | 'open' {
    if (!cfg.required || result == null) {
      this.uploadAllowed = true
      return 'open'
    }

    if (result.match) {
      this.markOwnerMatch()
      return 'owner'
    }

    this.rejectStreak++

    const ownerRecentMs = cfg.ownerRecentMs ?? 8000
    const ownerStillActive = Date.now() - this.lastOwnerMatchAt < ownerRecentMs

    if (ownerStillActive) {
      // 场景②：主人会话中夹杂他人声 → 静默过滤
      this.uploadAllowed = false
      return 'filter_foreign'
    }

    // 窗口外且无主人匹配：不上传，可能触发场景①
    this.uploadAllowed = false
    return 'foreign_only'
  }

  /** 当前 chunk 是否允许 upload（sendAudio）。 */
  shouldAllowUpload(): boolean {
    return this.uploadAllowed
  }

  /** 场景①：是否应触发 non_owner_turn（带冷却）。immediate=true 时唤醒 probe 一次失败即可。 */
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

/** 全局单例：与 realtimeStore 会话绑定。 */
export const speakerGate = new SpeakerGate()
