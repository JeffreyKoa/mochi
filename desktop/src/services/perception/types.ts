/**
 * Phase E：感知编排层公共类型。
 * PerceptionPhase 对应方案 v2 §2.1 轮次相位；TurnPhase 为 store 对外 UI 相位。
 */

import type { VisionFrameReason } from '@/services/visionCapture'

/** 方案 v2 感知相位（Orchestrator 内部 FSM）。 */
export type PerceptionPhase =
  | 'idle'
  | 'resting'
  | 'listen' // LISTEN：主人说话
  | 'endpoint' // ENDPOINT：submit 发帧窗口
  | 'think' // THINK：Mochi 思考
  | 'speak' // SPEAK：TTS 播放

/** realtimeStore 对外相位（与 UI / Dev 面板一致）。 */
export type TurnPhase = 'idle' | 'resting' | 'user_speaking' | 'processing' | 'agent_speaking'

export function turnPhaseToPerception(turn: TurnPhase): PerceptionPhase {
  switch (turn) {
    case 'idle':
      return 'idle'
    case 'resting':
      return 'resting'
    case 'user_speaking':
      return 'listen'
    case 'processing':
      return 'think'
    case 'agent_speaking':
      return 'speak'
  }
}

/** 相位 × 帧 reason 门控（§2.1 / §2.5：THINK 仅 glance Tier-0）。 */
export function perceptionAllowsVision(reason: VisionFrameReason, phase: PerceptionPhase): boolean {
  switch (reason) {
    case 'speech_start':
    case 'pause_probe':
      return phase === 'listen'
    case 'object_refresh':
    case 'audio_end':
      return phase === 'listen' || phase === 'endpoint'
    case 'glance':
      return phase === 'think'
    default:
      return false
  }
}

export interface FaceProbe {
  match: boolean
  score: number
  detected: boolean
}

export type PerceptionEventType =
  | 'phase_change'
  | 'vision_skip'
  | 'vision_sent'
  | 'vision_capture_failed'
  | 'turn_reset'

export interface PerceptionEvent {
  type: PerceptionEventType
  phase: PerceptionPhase
  reason?: string
  detail?: Record<string, unknown>
}

/** submit 前硬焦点 JPEG 发送窗口（ms）。 */
export const HARD_FOCUS_SEND_MS = 150

/** 举物 partial 命中后补拍延迟（ms）。 */
export const OBJECT_FRAME_HOLD_MS = 400

/** 静音≥此值触发 pause_probe（与 turnEndArbiter 一致）。 */
export { PAUSE_PROBE_MS } from '@/services/turnEndArbiter'
