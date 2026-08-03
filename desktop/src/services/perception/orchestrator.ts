/**
 * Phase E — PerceptionOrchestrator：Turn Phase 驱动的事件总线，协调 Eye / Ear Channel。
 */

import { getClientConfig } from '@/config'
import { evaluateTurnEnd } from '@/services/turnEndArbiter'
import { looksLikeObjectQuery } from '@/services/visionHeuristic'
import type { VisionFrameReason } from '@/services/visionCapture'
import { buildTurnEndSignals, type EarChannelTurnState } from './earChannel'
import { EyeChannel, type EyeChannelDeps, type EyeSendMeta } from './eyeChannel'
import {
  OBJECT_FRAME_HOLD_MS,
  turnPhaseToPerception,
  type PerceptionEvent,
  type PerceptionPhase,
  type TurnPhase,
} from './types'

export interface PerceptionOrchestratorDeps {
  getAbortSignal: () => AbortSignal | undefined
  sendVisionFrame: (b64: string, meta: EyeSendMeta) => boolean
  probeFaceFromJPEG: (buf: ArrayBuffer) => Promise<import('./types').FaceProbe | null>
  faceprintReady: () => boolean
  /** 周期验脸回调（EarChannel 触发，store 注入 probe 实现）。 */
  runPeriodicFaceCheck?: () => void | Promise<void>
  onEvent?: (ev: PerceptionEvent) => void
}

export class PerceptionOrchestrator {
  private phase: PerceptionPhase = 'idle'
  private speechStartFrameSent = false
  private objectRefreshCountThisTurn = 0
  private pauseProbeDone = false
  private pauseProbeInFlight = false

  readonly eye: EyeChannel

  constructor(private deps: PerceptionOrchestratorDeps) {
    const eyeDeps: EyeChannelDeps = {
      getPhase: () => this.phase,
      getAbortSignal: () => deps.getAbortSignal(),
      sendVisionFrame: deps.sendVisionFrame,
      probeFaceFromJPEG: deps.probeFaceFromJPEG,
      faceprintReady: deps.faceprintReady,
      onSkip: (skipReason, trigger) => {
        if (import.meta.env.DEV && skipReason !== 'config_speech_start_off') {
          console.info('[vision] skip_send reason=%s trigger=%s', skipReason, trigger)
        }
        deps.onEvent?.({ type: 'vision_skip', phase: this.phase, reason: skipReason, detail: { trigger } })
      },
    }
    this.eye = new EyeChannel(eyeDeps)
  }

  getPerceptionPhase(): PerceptionPhase {
    return this.phase
  }

  /** 与 store setPhase 同步（endpoint 由 enterEndpoint 单独设置）。 */
  syncTurnPhase(turn: TurnPhase): void {
    if (this.phase === 'endpoint' && turn === 'processing') {
      this.phase = 'think'
    } else {
      this.phase = turnPhaseToPerception(turn)
    }
    this.emit({ type: 'phase_change', phase: this.phase, detail: { turn } })
  }

  /** ENDPOINT：submit 发帧窗口（listen → endpoint）。 */
  enterEndpoint(): void {
    this.phase = 'endpoint'
    this.emit({ type: 'phase_change', phase: this.phase, reason: 'enter_endpoint' })
  }

  resetTurn(): void {
    this.speechStartFrameSent = false
    this.objectRefreshCountThisTurn = 0
    this.pauseProbeDone = false
    this.pauseProbeInFlight = false
    this.emit({ type: 'turn_reset', phase: this.phase })
  }

  isPauseProbeDone(): boolean {
    return this.pauseProbeDone
  }

  markPauseProbeDone(): void {
    this.pauseProbeDone = true
  }

  /** speech_start 预拍（每 turn 一次）。 */
  onSpeechStart(): void {
    if (this.speechStartFrameSent) return
    this.speechStartFrameSent = true
    void this.eye.sendFrame('speech_start')
  }

  /** ASR partial 举物语义 mid-turn 补拍。 */
  trackObjectIntentFromPartial(text: string): void {
    if (!text.trim() || this.objectRefreshCountThisTurn > 0) return
    if (!getClientConfig().visionSnapshotOnObjectIntent) return
    if (!looksLikeObjectQuery(text)) return
    this.objectRefreshCountThisTurn++
    setTimeout(() => {
      void this.eye.sendFrame('object_refresh')
    }, OBJECT_FRAME_HOLD_MS)
  }

  /** THINK 阶段 Tier-0 GLANCE。 */
  runThinkGlance(): void {
    if (this.phase !== 'think' || this.deps.getAbortSignal()?.aborted) return
    void this.eye.sendFrame('glance')
  }

  /** 静音≥3s：pause_probe + 可选周期验脸。 */
  async runPauseProbe(partial: string): Promise<void> {
    if (this.pauseProbeInFlight || this.phase !== 'listen') return
    this.pauseProbeInFlight = true
    try {
      await this.eye.sendFrame('pause_probe', partial)
      void this.deps.runPeriodicFaceCheck?.()
    } finally {
      this.pauseProbeInFlight = false
    }
  }

  /** submit 前视觉帧（含硬焦点 Barrier）。 */
  async prepareBeforeSubmit(turnText: string): Promise<void> {
    this.enterEndpoint()
    await this.eye.prepareBeforeSubmit(turnText)
  }

  /** 单帧发送。 */
  sendVisionFrame(reason: VisionFrameReason, asrPartial = ''): Promise<boolean> {
    return this.eye.sendFrame(reason, asrPartial)
  }

  /** 构建 turnEndArbiter 信号。 */
  buildTurnEndSignals(state: EarChannelTurnState) {
    return buildTurnEndSignals(state)
  }

  evaluateTurnEnd(state: EarChannelTurnState) {
    return evaluateTurnEnd(buildTurnEndSignals(state))
  }

  private emit(ev: PerceptionEvent): void {
    this.deps.onEvent?.(ev)
  }
}

export function createPerceptionOrchestrator(deps: PerceptionOrchestratorDeps): PerceptionOrchestrator {
  return new PerceptionOrchestrator(deps)
}

export { buildTurnEndSignals, shouldStartPeriodicFaceCheck, shouldStartStreamCheck } from './earChannel'
export { perceptionAllowsVision, turnPhaseToPerception } from './types'
export type { PerceptionPhase, TurnPhase, PerceptionEvent, FaceProbe } from './types'
