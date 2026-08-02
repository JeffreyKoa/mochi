import { describe, expect, it } from 'vitest'
import { IdentityGate } from '@/services/identityGate'
import type { RealtimeFaceprintConfig, RealtimeVoiceprintConfig } from '@/config'

const voiceCfg: RealtimeVoiceprintConfig = {
  required: true,
  threshold: 0.38,
  verifyWindowSec: 4,
  wakeProbeSec: 1,
  streamCheckIntervalMs: 500,
  rejectStreak: 3,
  ownerRecentMs: 8000,
  nonOwnerReplyCooldownMs: 12000,
}

const faceCfg: RealtimeFaceprintConfig = {
  enabled: true,
  required: false,
  matchThreshold: 0.42,
  grayZoneLow: 0.28,
  probeOnSpeechStart: true,
  checkIntervalMs: 2000,
  ownerRecentMs: 8000,
  enrollSamples: 3,
}

describe('IdentityGate', () => {
  it('voice match → owner', () => {
    const gate = new IdentityGate()
    const mode = gate.applyIdentityResult({ match: true, score: 0.5 }, null, voiceCfg, faceCfg)
    expect(mode).toBe('owner')
    expect(gate.shouldAllowUpload()).toBe(true)
  })

  it('voice gray + face match → owner_boost', () => {
    const gate = new IdentityGate()
    const mode = gate.applyIdentityResult(
      { match: false, score: 0.32 },
      { match: true, score: 0.5, detected: true },
      voiceCfg,
      faceCfg,
    )
    expect(mode).toBe('owner_boost')
    expect(gate.shouldAllowUpload()).toBe(true)
  })

  it('voice gray + face high score (no match) → owner_boost', () => {
    const gate = new IdentityGate()
    const mode = gate.applyIdentityResult(
      { match: false, score: 0.32 },
      { match: false, score: 0.39, detected: true },
      voiceCfg,
      faceCfg,
    )
    expect(mode).toBe('owner_boost')
  })

  it('voice fail + face owner → still foreign_only when no recent owner', () => {
    const gate = new IdentityGate()
    const mode = gate.applyIdentityResult(
      { match: false, score: 0.1 },
      { match: true, score: 0.9, detected: true },
      voiceCfg,
      faceCfg,
    )
    expect(mode).toBe('foreign_only')
    expect(gate.shouldAllowUpload()).toBe(false)
  })

  it('owner window + voice fail + no face → filter_foreign', () => {
    const gate = new IdentityGate()
    gate.markOwnerMatch()
    const mode = gate.applyIdentityResult(
      { match: false, score: 0.1 },
      null,
      voiceCfg,
      faceCfg,
    )
    expect(mode).toBe('filter_foreign')
    expect(gate.shouldAllowUpload()).toBe(false)
  })

  it('owner window + voice fail + face owner → owner_boost (keep upload)', () => {
    const gate = new IdentityGate()
    gate.markOwnerMatch()
    const mode = gate.applyIdentityResult(
      { match: false, score: 0.1 },
      { match: true, score: 0.8, detected: true },
      voiceCfg,
      faceCfg,
    )
    expect(mode).toBe('owner_boost')
    expect(gate.shouldAllowUpload()).toBe(true)
  })
})
