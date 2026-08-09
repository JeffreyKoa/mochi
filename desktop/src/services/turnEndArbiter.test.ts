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
    expect(isUnfinishedSpeech('你说喝什么啤酒好呢？雪花还是')).toBe(true)
  })

  it('x-asr speech_end blocks unfinished connective 还是', () => {
    const now = 10_000
    const partialUpdatedAt = now - 900
    const d = evaluateTurnEnd(
      base({
        vadSpeaking: true,
        partialText: '你说喝什么啤酒好呢？雪花还是',
        partialUpdatedAt,
        lastSpeechAt: partialUpdatedAt,
        speechEndedAt: now - 500,
        speechEndSubmitMs: 400,
        partialStableMs: 800,
        minCompleteSilenceMs: 700,
        unfinishedSilenceMs: 2200,
        silenceMsConfig: 600,
        disablePauseProbe: true,
        now,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).not.toBe('xasr_speech_end')
  })

  it('x-asr speech_end submits complete sentence after fast silence', () => {
    const now = 10_000
    const partialUpdatedAt = now - 900
    const d = evaluateTurnEnd(
      base({
        vadSpeaking: true,
        partialText: '我晚上想喝点啤酒',
        partialUpdatedAt,
        lastSpeechAt: partialUpdatedAt,
        speechEndedAt: now - 500,
        speechEndSubmitMs: 400,
        partialStableMs: 800,
        minCompleteSilenceMs: 700,
        silenceMsConfig: 600,
        disablePauseProbe: true,
        now,
      }),
    )
    expect(d.ready).toBe(true)
    expect(d.reason).toBe('xasr_speech_end')
  })

  it('unfinished long tail waits for unfinished_silence_ms', () => {
    const now = 10_000
    const partialUpdatedAt = now - 900
    const d = evaluateTurnEnd(
      base({
        partialText: '我就不是夸你说话有特色吧，我觉得像就是像白',
        partialUpdatedAt,
        lastSpeechAt: partialUpdatedAt,
        speechEndedAt: now - 500,
        speechEndSubmitMs: 400,
        partialStableMs: 800,
        minCompleteSilenceMs: 700,
        unfinishedSilenceMs: 2200,
        silenceMsConfig: 600,
        disablePauseProbe: true,
        now,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).toBe('silence_short')
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

  it('empty partial with many chunks blocks submit (no ASR text)', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '',
        partialUpdatedAt: 0,
        lastSpeechAt: 10_000 - 1500,
        chunksSent: 100,
        now: 10_000,
      }),
    )
    expect(d.ready).toBe(false)
    expect(d.reason).toBe('no_partial')
  })

  it('empty partial with few chunks can still submit after silence', () => {
    const d = evaluateTurnEnd(
      base({
        partialText: '',
        partialUpdatedAt: 0,
        lastSpeechAt: 10_000 - 1500,
        chunksSent: 20,
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

  it('x-asr partial without VAD peak can submit after silence', () => {
    const partialUpdatedAt = 10_000 - 3000
    const d = evaluateTurnEnd(
      base({
        partialText: '你好呀今天天气怎么样',
        partialUpdatedAt,
        lastSpeechAt: 0,
        now: 10_000,
        partialStableMs: 300,
        minCompleteSilenceMs: 450,
        silenceMsConfig: 600,
        disablePauseProbe: true,
      }),
    )
    expect(d.ready).toBe(true)
    expect(d.reason).toBe('ready')
  })

  it('x-asr speech_end can submit while vad still in redemption', () => {
    const now = 10_000
    const partialUpdatedAt = now - 900
    const d = evaluateTurnEnd(
      base({
        vadSpeaking: true,
        partialText: '小尾巴为什么你不回应',
        partialUpdatedAt,
        lastSpeechAt: partialUpdatedAt,
        speechEndedAt: now - 500,
        speechEndSubmitMs: 400,
        partialStableMs: 800,
        minCompleteSilenceMs: 700,
        silenceMsConfig: 600,
        disablePauseProbe: true,
        now,
      }),
    )
    expect(d.ready).toBe(true)
    expect(d.reason).toBe('xasr_speech_end')
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
