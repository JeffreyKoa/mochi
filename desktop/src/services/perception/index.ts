export {
  createPerceptionOrchestrator,
  PerceptionOrchestrator,
  perceptionAllowsVision,
  turnPhaseToPerception,
  buildTurnEndSignals,
  shouldStartPeriodicFaceCheck,
  shouldStartStreamCheck,
} from './orchestrator'
export type { PerceptionPhase, TurnPhase, PerceptionEvent, FaceProbe } from './types'
export { shouldSendVisionFrame } from './eyeChannel'
export { PAUSE_PROBE_MS, HARD_FOCUS_SEND_MS, OBJECT_FRAME_HOLD_MS } from './types'
