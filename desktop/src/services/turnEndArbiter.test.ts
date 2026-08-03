import { describe, expect, it } from 'vitest'
import {
  evaluateTurnEnd,
  isUnfinishedSpeech,
  PAUSE_PROBE_MS,
  PARTIAL_STABLE_MS,
  THINKING_HOLD_EXTEND_MS,
  UNFINISHED_SILENCE_MS,
} from './turnEndArbiter'

const base = (over: Partial<Parameters<typeof evaluateTurnEnd>[0]> = {}) => ({
  heardSpeech: true,
  lastSpeechAt: 0,
  vadSpeaking: false,
  partialText: '',
  partialUpdatedAt: 0,
  chunksSent: 100,
  thinkingHoldUntil: 0,
  silenceMsConfig: 1400,
  resumedAfterPauseProbe: false,
  now: 10_000,
  ...over,
})

describe('turnEndArbiter', () => {
  it('mid-pause on 因为 extends hold once path', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '因为',
        partialUpdatedAt: 5000,
        lastSpeechAt: 10_000 - PAUSE_PROBE_MS,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).toBe('pause_unfinished')
    expect(d.extendHoldMs).toBe(THINKING_HOLD_EXTEND_MS)
  })

  it('after resume, final silence can submit', () => {
    const lastSpeechAt = 10_000 - UNFINISHED_SILENCE_MS - 100
    const d = evaluateTurnEnd(
      base({
        partialText: '因为天气不好然后就没出门',
        partialUpdatedAt: lastSpeechAt,
        lastSpeechAt,
        resumedAfterPauseProbe: true,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(true)
    expect(d.reason).toBe('ready')
  })

  it('does not treat normal length ASR as unfinished', () => {
    expect(isUnfinishedSpeech('你今天在做什么呢')).toBe(false)
    expect(isUnfinishedSpeech('因为')).toBe(true)
  })

  it('blocks while partial still changing', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '你好',
        partialUpdatedAt: 10_000 - 500,
        lastSpeechAt: 10_000 - 2000,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).toBe('partial_unstable')
  })

  it('empty partial is not treated as unfinished short sentence', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '',
        partialUpdatedAt: 0,
        lastSpeechAt: 10_000 - 1500,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(true)
  })

  it('pause_hint_composing extends hold on mid pause', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '因为',
        partialUpdatedAt: 5000,
        lastSpeechAt: 10_000 - PAUSE_PROBE_MS,
        pauseHintComposing: true,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).toBe('pause_hint_composing')
    expect(d.extendHoldMs).toBe(THINKING_HOLD_EXTEND_MS)
  })

  it('respects thinking_hold', () => {
    const d = evaluateTurnEnd(
      base({
        thinkingHoldUntil: 20_000,
        partialText: '完整的一句话',
        partialUpdatedAt: 5000,
        lastSpeechAt: 10_000 - 5000,
        now: 10_000,
      }),
    )
    expect(d.reason).toBe('thinking_hold')
  })
})
