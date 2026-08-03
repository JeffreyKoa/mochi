import { describe, expect, it, vi, beforeEach } from 'vitest'
import { shouldSendVisionFrame } from './eyeChannel'
import {
  PerceptionOrchestrator,
  perceptionAllowsVision,
  turnPhaseToPerception,
} from './orchestrator'

describe('perceptionAllowsVision', () => {
  it('glance only in think', () => {
    expect(perceptionAllowsVision('glance', 'think')).toBe(true)
    expect(perceptionAllowsVision('glance', 'listen')).toBe(false)
    expect(perceptionAllowsVision('glance', 'speak')).toBe(false)
  })

  it('pause_probe only in listen', () => {
    expect(perceptionAllowsVision('pause_probe', 'listen')).toBe(true)
    expect(perceptionAllowsVision('pause_probe', 'think')).toBe(false)
  })

  it('audio_end in listen and endpoint', () => {
    expect(perceptionAllowsVision('audio_end', 'listen')).toBe(true)
    expect(perceptionAllowsVision('audio_end', 'endpoint')).toBe(true)
    expect(perceptionAllowsVision('audio_end', 'think')).toBe(false)
  })
})

describe('turnPhaseToPerception', () => {
  it('maps store phases', () => {
    expect(turnPhaseToPerception('user_speaking')).toBe('listen')
    expect(turnPhaseToPerception('processing')).toBe('think')
    expect(turnPhaseToPerception('agent_speaking')).toBe('speak')
  })
})

describe('PerceptionOrchestrator', () => {
  const sendVisionFrame = vi.fn(() => true)
  const probeFaceFromJPEG = vi.fn(async () => null)

  beforeEach(() => {
    sendVisionFrame.mockClear()
    probeFaceFromJPEG.mockClear()
  })

  function makeOrch() {
    return new PerceptionOrchestrator({
      getAbortSignal: () => undefined,
      sendVisionFrame,
      probeFaceFromJPEG,
      faceprintReady: () => false,
    })
  }

  it('resetTurn clears pause probe state', () => {
    const orch = makeOrch()
    orch.markPauseProbeDone()
    orch.resetTurn()
    expect(orch.isPauseProbeDone()).toBe(false)
  })

  it('enterEndpoint then sync processing becomes think', () => {
    const orch = makeOrch()
    orch.syncTurnPhase('user_speaking')
    orch.enterEndpoint()
    expect(orch.getPerceptionPhase()).toBe('endpoint')
    orch.syncTurnPhase('processing')
    expect(orch.getPerceptionPhase()).toBe('think')
  })

  it('runThinkGlance blocked outside think', () => {
    const orch = makeOrch()
    orch.syncTurnPhase('user_speaking')
    orch.runThinkGlance()
    expect(sendVisionFrame).not.toHaveBeenCalled()
  })
})

describe('shouldSendVisionFrame phase gate', () => {
  it('blocks glance in listen', () => {
    const gate = shouldSendVisionFrame('glance', 'listen')
    expect(gate.ok).toBe(false)
    expect(gate.skipReason).toBe('phase_blocked')
  })
})
