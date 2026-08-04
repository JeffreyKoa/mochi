/**
 * 多模态轮次结束仲裁（眼耳协同 · Phase1）
 *
 * 「耳」：VAD 静音时长、ASR partial 是否仍在变化
 * 「眼」：pause_probe 帧（>3s 停顿时抓拍，供 VL 更新表情；Phase2 可回传 thinking）
 *
 * 仅在 evaluateTurnEnd → ready 时客户端才 sendAudioEnd，避免 Mochi 抢话。
 */

/** 与 realtimeStore 中 UNFINISHED_CONNECTIVES 保持一致 */
const UNFINISHED_CONNECTIVES = [
  '但是', '但是呢', '因为', '所以', '然后', '而且', '如果', '不过',
  '虽然', '觉得', '特别是', '比如', '并且', '另外', '还有', '其实',
  '或者', '结果', '就是', '就是说', '意思是', '也就是说', '然后呢', '所以说',
  '……', '...', '---', '、',
]

/** 停顿时长达到此值触发「眼」probe（抓拍 + 延长等待） */
export const PAUSE_PROBE_MS = 3000

/** 判定「还在组织语言」时额外等待 */
export const THINKING_HOLD_EXTEND_MS = 4500

/** ASR partial 稳定多久才认为不再更新 */
export const PARTIAL_STABLE_MS = 1500

/** 最短句末静音（完整句） */
export const MIN_COMPLETE_SILENCE_MS = 1400

/** 未完成句所需静音更长 */
export const UNFINISHED_SILENCE_MS = 2800

export interface TurnEndSignals {
  heardSpeech: boolean
  lastSpeechAt: number
  vadSpeaking: boolean
  partialText: string
  partialUpdatedAt: number
  chunksSent: number
  thinkingHoldUntil: number
  silenceMsConfig: number
  /** 3s pause_probe 之后用户是否已继续说话（续说后不再按「句中停顿」延长） */
  resumedAfterPauseProbe?: boolean
  /** Tier-0 vision_pause_hint：服务端推断仍在组织语言 */
  pauseHintComposing?: boolean
  /** 本地 X-ASR：跳过 3s 眼动 pause_probe 延长 */
  disablePauseProbe?: boolean
  /** 覆盖默认 partial 稳定窗口 */
  partialStableMs?: number
  minCompleteSilenceMs?: number
  unfinishedSilenceMs?: number
  /** VAD speech_end 时刻（本地 X-ASR 加速提交） */
  speechEndedAt?: number
  speechEndSubmitMs?: number
  now?: number
}

export interface TurnEndDecision {
  ready: boolean
  reason: string
  /** 建议延长 thinkingHoldUntil（pause probe 发现可能还在想） */
  extendHoldMs?: number
  /** 是否应触发 pause_probe 抓拍 */
  shouldPauseProbe?: boolean
}

export function isUnfinishedSpeech(text: string): boolean {
  const trimmed = text.trim()
  if (!trimmed) return false
  for (const conn of UNFINISHED_CONNECTIVES) {
    if (trimmed.endsWith(conn)) return true
  }
  const lastChar = trimmed.slice(-1)
  if (lastChar === '，' || lastChar === ',' || lastChar === '：' || lastChar === ':') {
    return true
  }
  // 中文 ASR 通常不带句号；仅当明显过短且无句末标点时才视为可能未完
  if (trimmed.length < 8 && !/[。！？?!]$/.test(trimmed)) {
    return true
  }
  return false
}

/** 最近一次「有语音/识别活动」的时间戳（X-ASR partial 可能先于 VAD 能量更新）。 */
function speechActivityAt(signals: TurnEndSignals): number {
  const partialAt = signals.partialText.trim() ? signals.partialUpdatedAt : 0
  return Math.max(signals.lastSpeechAt, partialAt)
}

/** 多模态轮次结束：是否可 submit（sendAudioEnd） */
export function evaluateTurnEnd(signals: TurnEndSignals): TurnEndDecision {
  const now = signals.now ?? Date.now()

  const partialStableMs = signals.partialStableMs ?? PARTIAL_STABLE_MS
  const minCompleteSilenceMs = signals.minCompleteSilenceMs ?? MIN_COMPLETE_SILENCE_MS
  const unfinishedSilenceMs = signals.unfinishedSilenceMs ?? UNFINISHED_SILENCE_MS
  const pauseProbeMs = signals.disablePauseProbe ? Number.MAX_SAFE_INTEGER : PAUSE_PROBE_MS

  const activityAt = speechActivityAt(signals)
  if (!signals.heardSpeech || activityAt <= 0) {
    return { ready: false, reason: 'no_speech_yet' }
  }
  if (signals.vadSpeaking) {
    return { ready: false, reason: 'vad_speaking' }
  }
  if (now < signals.thinkingHoldUntil) {
    return { ready: false, reason: 'thinking_hold' }
  }

  const silence = now - activityAt
  const partial = signals.partialText.trim()
  /** 判定句中可能未完（空 partial 不算未完，避免 ASR 延迟误拦） */
  const unfinished =
    isUnfinishedSpeech(partial) || (partial.length > 0 && partial.length < 12)

  // ASR 仍在更新 → 嘴还在动（流式延迟），不提交
  if (partial && now - signals.partialUpdatedAt < partialStableMs) {
    return { ready: false, reason: 'partial_unstable' }
  }

  // 本地 X-ASR：VAD 已 speech_end 且 partial 稳定 → 更短路径提交
  if (
    signals.speechEndedAt &&
    signals.speechEndSubmitMs &&
    !signals.vadSpeaking &&
    partial &&
    now - signals.speechEndedAt >= signals.speechEndSubmitMs &&
    now - signals.partialUpdatedAt >= partialStableMs
  ) {
    const silenceSinceSpeech = now - activityAt
    const fastSilence = Math.min(signals.silenceMsConfig, minCompleteSilenceMs)
    if (silenceSinceSpeech >= fastSilence) {
      return { ready: true, reason: 'xasr_speech_end' }
    }
  }

  // >3s 停顿且尚未续说：视为句中组织语言，延长等待（仅 pause_probe 后、续说前）
  const midPause = silence >= pauseProbeMs && !signals.resumedAfterPauseProbe
  if (midPause && signals.pauseHintComposing && unfinished) {
    return {
      ready: false,
      reason: 'pause_hint_composing',
      extendHoldMs: THINKING_HOLD_EXTEND_MS,
    }
  }
  if (midPause && unfinished) {
    return {
      ready: false,
      reason: 'pause_unfinished',
      extendHoldMs: THINKING_HOLD_EXTEND_MS,
      shouldPauseProbe: true,
    }
  }
  if (midPause && !partial && signals.chunksSent > 80) {
    return {
      ready: false,
      reason: 'pause_no_partial',
      extendHoldMs: THINKING_HOLD_EXTEND_MS,
      shouldPauseProbe: true,
    }
  }

  const requiredSilence = unfinished
    ? unfinishedSilenceMs
    : Math.max(signals.silenceMsConfig, minCompleteSilenceMs)

  if (silence < requiredSilence) {
    if (!signals.disablePauseProbe && silence >= PAUSE_PROBE_MS) {
      return { ready: false, reason: 'silence_short', shouldPauseProbe: true }
    }
    return { ready: false, reason: 'silence_short' }
  }

  return { ready: true, reason: 'ready' }
}

/** VL 表情是否表示仍在组织语言（Phase2 服务端回传后可接） */
export function isComposingExpression(expression: string): boolean {
  const e = expression.toLowerCase().trim()
  return e === 'thinking' || e === 'hesitant' || e === 'anxious'
}
