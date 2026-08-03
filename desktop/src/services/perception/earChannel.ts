/**
 * Phase E — 耳（EarChannel）：stream_check / 周期验脸策略与 turn 结束信号辅助。
 */

import { getFaceprintConfig, getVoiceprintConfig } from '@/config'
import { shouldRunPeriodicFaceCheck } from '@/services/lowPowerMode'
import type { TurnEndSignals } from '@/services/turnEndArbiter'
import type { PerceptionPhase, TurnPhase } from './types'

export interface EarChannelTurnState {
  heardSpeech: boolean
  lastSpeechAt: number
  vadSpeaking: boolean
  partialText: string
  partialUpdatedAt: number
  chunksSent: number
  thinkingHoldUntil: number
  silenceMsConfig: number
  resumedAfterPauseProbe: boolean
  pauseHintComposing: boolean
}

/** 是否应启动对话中 stream_check 定时器。 */
export function shouldStartStreamCheck(opts: {
  voiceprintRequired: boolean
  voiceprintReady: boolean
  sttMode: 'cloud' | 'local'
}): boolean {
  if (opts.sttMode === 'local') return false
  if (!opts.voiceprintRequired) return false
  return opts.voiceprintReady
}

export function getStreamCheckIntervalMs(): number {
  return getVoiceprintConfig().streamCheckIntervalMs || 500
}

export function getStreamCheckGraceMs(): number {
  return 1200
}

/** 是否启用 LISTEN 内周期性 ONNX 验脸。 */
export function shouldStartPeriodicFaceCheck(opts: {
  faceprintEnabled: boolean
  faceprintReady: boolean
  phase: TurnPhase
  recording: boolean
}): boolean {
  if (!shouldRunPeriodicFaceCheck()) return false
  if (!opts.faceprintEnabled || !opts.faceprintReady) return false
  if (opts.phase !== 'user_speaking' || !opts.recording) return false
  return true
}

export function getPeriodicFaceCheckIntervalMs(): number {
  return getFaceprintConfig().checkIntervalMs || 2000
}

/** 从 store 快照构建 turnEndArbiter 输入。 */
export function buildTurnEndSignals(state: EarChannelTurnState): TurnEndSignals {
  return {
    heardSpeech: state.heardSpeech,
    lastSpeechAt: state.lastSpeechAt,
    vadSpeaking: state.vadSpeaking,
    partialText: state.partialText,
    partialUpdatedAt: state.partialUpdatedAt,
    chunksSent: state.chunksSent,
    thinkingHoldUntil: state.thinkingHoldUntil,
    silenceMsConfig: state.silenceMsConfig,
    resumedAfterPauseProbe: state.resumedAfterPauseProbe,
    pauseHintComposing: state.pauseHintComposing,
  }
}

/** stream_check / 周期验脸允许的感知相位。 */
export function earChannelActivePhase(phase: PerceptionPhase): boolean {
  return phase === 'listen'
}
