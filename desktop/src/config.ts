let _apiBase = ''

export interface RealtimeSileroVadConfig {
  positiveThreshold: number
  negativeThreshold: number
  redemptionMs: number
  minSpeechMs: number
  preSpeechPadMs: number
}

export interface RealtimeVadConfig {
  silenceMs: number
  minSpeechMs: number
  endpointingEnabled: boolean
  energyPeak: number
  tailSpeechPeak: number
  playbackPeak: number
  wakePeak: number
  silero: RealtimeSileroVadConfig
}

export interface RealtimeBargeInConfig {
  echoGuardMs: number
  peakThreshold: number
  bargeInMs: number
}

export interface RealtimeVoiceprintConfig {
  required: boolean
  threshold: number
  verifyWindowSec: number
  wakeProbeSec: number
  streamCheckIntervalMs: number
  /** 连续验声失败多少次视为非主人（场景①）。 */
  rejectStreak: number
  /** 此窗口内验失败 → 场景②静默过滤。 */
  ownerRecentMs: number
  /** 场景①拒答 TTS 冷却（毫秒）。 */
  nonOwnerReplyCooldownMs: number
}

/** P2 主人人脸认人配置 */
export interface RealtimeFaceprintConfig {
  enabled: boolean
  required: boolean
  matchThreshold: number
  grayZoneLow: number
  probeOnSpeechStart: boolean
  checkIntervalMs: number
  ownerRecentMs: number
  enrollSamples: number
}

export interface RealtimePresenceConfig {
  enabled: boolean
  ambientIntervalMs: number
  awayTimeoutSec: number
  speechThreshold: number
  ambientEnergyFloor: number
  ownerPresenceTtlSec: number
}

export type SttMode = 'cloud' | 'local' | 'auto'
export type TtsMode = 'cloud' | 'local' | 'auto'

/** X-ASR 本地 sidecar（客户端优先 POC）。 */
export interface RealtimeXasrConfig {
  /** 是否尝试连接本地 X-ASR WebSocket。 */
  enabled: boolean
  wsUrl: string
  /** 客户端发送 PCM 的聚合块时长（毫秒）。 */
  chunkMs: number
  /** 句末静音达到此值可提交（本地比云端更短）。 */
  silenceMs: number
  /** partial 稳定多久视为不再更新。 */
  partialStableMs: number
  /** 完整句最短句末静音。 */
  minCompleteSilenceMs: number
  /** 疑似未完句所需静音。 */
  unfinishedSilenceMs: number
  /** VAD speech_end 后加速提交：距 speech_end 至少此毫秒且 partial 已稳定。 */
  speechEndSubmitMs: number
}

/** X-TTS Matcha sidecar（本地 TTS POC）。 */
export interface RealtimeXttsConfig {
  /** 是否尝试连接本地 X-TTS HTTP sidecar。 */
  enabled: boolean
  baseUrl: string
  /** 合成语速。 */
  speed: number
}

export interface RealtimeClientConfig {
  sttMode: SttMode
  ttsMode: TtsMode
  speechLocale: string
  xasr: RealtimeXasrConfig
  xtts: RealtimeXttsConfig
  vad: RealtimeVadConfig
  bargeIn: RealtimeBargeInConfig
  voiceprint: RealtimeVoiceprintConfig
  faceprint: RealtimeFaceprintConfig
  presence: RealtimePresenceConfig
}

/** 视觉客户端配置（来自 public config vision 块）。 */
export interface VisionClientConfig {
  /** 会话级摄像头：startTalk 后保持流，默认 true */
  sessionCamera: boolean
  /** speech_start 时预拍一帧，默认 false（服务端可开） */
  snapshotOnSpeechStart: boolean
  /** audio_end 时抓拍，默认 true */
  snapshotOnAudioEnd: boolean
}

export interface CompanionPresenceConfig {
  enabled: boolean
  intervalSec: number
  cooldownMin: number
  dailyMax: number
}

export const DEFAULT_COMPANION_PRESENCE: CompanionPresenceConfig = {
  enabled: true,
  intervalSec: 45,
  cooldownMin: 45,
  dailyMax: 8,
}

export interface ClientConfig {
  apiBase: string
  realtimeEnabled: boolean
  writeApproval: boolean
  growthEnabled: boolean
  visionEnabled: boolean
  visionSessionCamera: boolean
  visionSnapshotOnSpeechStart: boolean
  /** P2：举物语义 detected 时 mid-turn / submit 前补拍 */
  visionSnapshotOnObjectIntent: boolean
  visionSnapshotOnAudioEnd: boolean
  /** Phase D：低配模式（降 FPS、限抓拍、关周期 ONNX） */
  lowPowerMode: boolean
  eventLoopProbeMs: number
  realtime: RealtimeClientConfig
  companionPresence: CompanionPresenceConfig
}

export const DEFAULT_SILERO: RealtimeSileroVadConfig = {
  positiveThreshold: 0.5,
  negativeThreshold: 0.35,
  redemptionMs: 1200,
  minSpeechMs: 350,
  preSpeechPadMs: 300,
}

export const DEFAULT_XASR: RealtimeXasrConfig = {
  enabled: true,
  wsUrl: 'ws://127.0.0.1:8766',
  chunkMs: 40,
  silenceMs: 600,
  partialStableMs: 300,
  minCompleteSilenceMs: 450,
  unfinishedSilenceMs: 900,
  speechEndSubmitMs: 200,
}

export const DEFAULT_XTTS: RealtimeXttsConfig = {
  enabled: true,
  baseUrl: 'http://127.0.0.1:8767',
  speed: 1.0,
}

export const DEFAULT_REALTIME: RealtimeClientConfig = {
  sttMode: 'auto',
  ttsMode: 'auto',
  speechLocale: 'zh-CN',
  xasr: { ...DEFAULT_XASR },
  xtts: { ...DEFAULT_XTTS },
  vad: {
    silenceMs: 1800,
    minSpeechMs: 250,
    endpointingEnabled: false,
    energyPeak: 0.05,
    tailSpeechPeak: 0.015,
    playbackPeak: 0.10,
    wakePeak: 0.06,
    silero: { ...DEFAULT_SILERO },
  },
  bargeIn: {
    echoGuardMs: 1800,
    peakThreshold: 0.06,
    bargeInMs: 800,
  },
  voiceprint: {
    required: true,
    threshold: 0.38,
    verifyWindowSec: 4.0,
    wakeProbeSec: 1.0,
    streamCheckIntervalMs: 500,
    rejectStreak: 3,
    ownerRecentMs: 8000,
    nonOwnerReplyCooldownMs: 12000,
  },
  faceprint: {
    enabled: true,
    required: false,
    matchThreshold: 0.42,
    grayZoneLow: 0.28,
    probeOnSpeechStart: true,
    checkIntervalMs: 2000,
    ownerRecentMs: 8000,
    enrollSamples: 3,
  },
  presence: {
    enabled: true,
    ambientIntervalMs: 2000,
    awayTimeoutSec: 180,
    speechThreshold: 0.3,
    ambientEnergyFloor: 0.012,
    ownerPresenceTtlSec: 30,
  },
}

let _clientConfig: ClientConfig = {
  apiBase: '',
  realtimeEnabled: true,
  writeApproval: false,
  growthEnabled: true,
  visionEnabled: false,
  visionSessionCamera: true,
  visionSnapshotOnSpeechStart: false,
  visionSnapshotOnObjectIntent: true,
  visionSnapshotOnAudioEnd: true,
  lowPowerMode: false,
  eventLoopProbeMs: 1000,
  realtime: { ...DEFAULT_REALTIME },
  companionPresence: { ...DEFAULT_COMPANION_PRESENCE },
}

function parseVisionBlock(data: Record<string, unknown>): Pick<
  ClientConfig,
  | 'visionSessionCamera'
  | 'visionSnapshotOnSpeechStart'
  | 'visionSnapshotOnObjectIntent'
  | 'visionSnapshotOnAudioEnd'
> {
  const v = (data.vision ?? {}) as Record<string, unknown>
  return {
    visionSessionCamera: v.session_camera !== false,
    visionSnapshotOnSpeechStart: !!v.snapshot_on_speech_start,
    visionSnapshotOnObjectIntent: v.snapshot_on_object_intent !== false,
    visionSnapshotOnAudioEnd: v.snapshot_on_audio_end !== false,
  }
}

export function getApiBase(): string {
  return _apiBase
}

export function getClientConfig(): ClientConfig {
  return _clientConfig
}

export function getRealtimeConfig(): RealtimeClientConfig {
  return _clientConfig.realtime
}

export function getVoiceprintConfig(): RealtimeVoiceprintConfig {
  return _clientConfig.realtime.voiceprint
}

export function getFaceprintConfig(): RealtimeFaceprintConfig {
  return _clientConfig.realtime.faceprint
}

export function getPresenceConfig(): RealtimePresenceConfig {
  return _clientConfig.realtime.presence
}

export function getCompanionPresenceConfig(): CompanionPresenceConfig {
  return _clientConfig.companionPresence
}

export function setApiBase(url: string) {
  _apiBase = url.replace(/\/$/, '')
  _clientConfig = { ..._clientConfig, apiBase: _apiBase }
}

function num(v: unknown, fallback: number): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback
}

function parseSileroBlock(raw: unknown, base: RealtimeSileroVadConfig): RealtimeSileroVadConfig {
  if (!raw || typeof raw !== 'object') return base
  const s = raw as Record<string, unknown>
  return {
    positiveThreshold: num(s.positive_threshold ?? s.positiveThreshold, base.positiveThreshold),
    negativeThreshold: num(s.negative_threshold ?? s.negativeThreshold, base.negativeThreshold),
    redemptionMs: num(s.redemption_ms ?? s.redemptionMs, base.redemptionMs),
    minSpeechMs: num(s.min_speech_ms ?? s.minSpeechMs, base.minSpeechMs),
    preSpeechPadMs: num(s.pre_speech_pad_ms ?? s.preSpeechPadMs, base.preSpeechPadMs),
  }
}

function parseRealtimeBlock(raw: unknown): RealtimeClientConfig {
  const base = {
    ...DEFAULT_REALTIME,
    xasr: { ...DEFAULT_REALTIME.xasr },
    xtts: { ...DEFAULT_REALTIME.xtts },
    vad: { ...DEFAULT_REALTIME.vad, silero: { ...DEFAULT_REALTIME.vad.silero } },
    bargeIn: { ...DEFAULT_REALTIME.bargeIn },
    voiceprint: { ...DEFAULT_REALTIME.voiceprint },
    faceprint: { ...DEFAULT_REALTIME.faceprint },
    presence: { ...DEFAULT_REALTIME.presence },
  }
  if (!raw || typeof raw !== 'object') return base

  const r = raw as Record<string, unknown>
  const mode = String(r.stt_mode ?? r.sttMode ?? base.sttMode)
  if (mode === 'cloud' || mode === 'local' || mode === 'auto') {
    base.sttMode = mode
  }

  const ttsMode = String(r.tts_mode ?? r.ttsMode ?? base.ttsMode)
  if (ttsMode === 'cloud' || ttsMode === 'local' || ttsMode === 'auto') {
    base.ttsMode = ttsMode
  }

  if (typeof r.speech_locale === 'string' && r.speech_locale) {
    base.speechLocale = r.speech_locale
  } else if (typeof r.speechLocale === 'string' && r.speechLocale) {
    base.speechLocale = r.speechLocale
  }

  const xasr = r.xasr
  if (xasr && typeof xasr === 'object') {
    const x = xasr as Record<string, unknown>
    base.xasr = {
      enabled: x.enabled !== false,
      wsUrl: String(x.ws_url ?? x.wsUrl ?? base.xasr.wsUrl),
      chunkMs: num(x.chunk_ms ?? x.chunkMs, base.xasr.chunkMs),
      silenceMs: num(x.silence_ms ?? x.silenceMs, base.xasr.silenceMs),
      partialStableMs: num(x.partial_stable_ms ?? x.partialStableMs, base.xasr.partialStableMs),
      minCompleteSilenceMs: num(
        x.min_complete_silence_ms ?? x.minCompleteSilenceMs,
        base.xasr.minCompleteSilenceMs,
      ),
      unfinishedSilenceMs: num(
        x.unfinished_silence_ms ?? x.unfinishedSilenceMs,
        base.xasr.unfinishedSilenceMs,
      ),
      speechEndSubmitMs: num(
        x.speech_end_submit_ms ?? x.speechEndSubmitMs,
        base.xasr.speechEndSubmitMs,
      ),
    }
  }

  const xtts = r.xtts
  if (xtts && typeof xtts === 'object') {
    const t = xtts as Record<string, unknown>
    base.xtts = {
      enabled: t.enabled !== false,
      baseUrl: String(t.base_url ?? t.baseUrl ?? base.xtts.baseUrl),
      speed: num(t.speed, base.xtts.speed),
    }
  }

  const vad = r.vad
  if (vad && typeof vad === 'object') {
    const v = vad as Record<string, unknown>
    base.vad = {
      silenceMs: num(v.silence_ms ?? v.silenceMs, base.vad.silenceMs),
      minSpeechMs: num(v.min_speech_ms ?? v.minSpeechMs, base.vad.minSpeechMs),
      endpointingEnabled: v.endpointing_enabled !== false && v.endpointingEnabled !== false,
      energyPeak: num(v.energy_peak ?? v.energyPeak, base.vad.energyPeak),
      tailSpeechPeak: num(v.tail_speech_peak ?? v.tailSpeechPeak, base.vad.tailSpeechPeak),
      playbackPeak: num(v.playback_peak ?? v.playbackPeak, base.vad.playbackPeak),
      wakePeak: num(v.wake_peak ?? v.wakePeak, 0),
      silero: parseSileroBlock(v.silero, base.vad.silero),
    }
  }

  const barge = r.barge_in ?? r.bargeIn
  if (barge && typeof barge === 'object') {
    const b = barge as Record<string, unknown>
    base.bargeIn = {
      echoGuardMs: num(b.echo_guard_ms ?? b.echoGuardMs, base.bargeIn.echoGuardMs),
      peakThreshold: num(b.peak_threshold ?? b.peakThreshold, base.bargeIn.peakThreshold),
      bargeInMs: num(b.barge_in_ms ?? b.bargeInMs, base.bargeIn.bargeInMs),
    }
  }

  if (base.vad.wakePeak <= 0) {
    base.vad.wakePeak = base.bargeIn.peakThreshold
  }

  const vp = r.voiceprint
  if (vp && typeof vp === 'object') {
    const v = vp as Record<string, unknown>
    base.voiceprint = {
      required: v.required !== false,
      threshold: num(v.threshold, base.voiceprint.threshold),
      verifyWindowSec: num(v.verify_window_sec ?? v.verifyWindowSec, base.voiceprint.verifyWindowSec),
      wakeProbeSec: num(v.wake_probe_sec ?? v.wakeProbeSec, base.voiceprint.wakeProbeSec),
      streamCheckIntervalMs: num(
        v.stream_check_interval_ms ?? v.streamCheckIntervalMs,
        base.voiceprint.streamCheckIntervalMs,
      ),
      rejectStreak: num(v.reject_streak ?? v.rejectStreak, base.voiceprint.rejectStreak),
      ownerRecentMs: num(v.owner_recent_ms ?? v.ownerRecentMs, base.voiceprint.ownerRecentMs),
      nonOwnerReplyCooldownMs: num(
        v.non_owner_reply_cooldown_ms ?? v.nonOwnerReplyCooldownMs,
        base.voiceprint.nonOwnerReplyCooldownMs,
      ),
    }
  }

  const fp = r.faceprint
  if (fp && typeof fp === 'object') {
    const f = fp as Record<string, unknown>
    base.faceprint = {
      enabled: f.enabled !== false,
      required: !!f.required,
      matchThreshold: num(f.match_threshold ?? f.matchThreshold, base.faceprint.matchThreshold),
      grayZoneLow: num(f.gray_zone_low ?? f.grayZoneLow, base.faceprint.grayZoneLow),
      probeOnSpeechStart: f.probe_on_speech_start !== false && f.probeOnSpeechStart !== false,
      checkIntervalMs: num(f.check_interval_ms ?? f.checkIntervalMs, base.faceprint.checkIntervalMs),
      ownerRecentMs: num(f.owner_recent_ms ?? f.ownerRecentMs, base.faceprint.ownerRecentMs),
      enrollSamples: num(f.enroll_samples ?? f.enrollSamples, base.faceprint.enrollSamples),
    }
  }

  const pr = r.presence
  if (pr && typeof pr === 'object') {
    const p = pr as Record<string, unknown>
    base.presence = {
      enabled: p.enabled !== false,
      ambientIntervalMs: num(p.ambient_interval_ms ?? p.ambientIntervalMs, base.presence.ambientIntervalMs),
      awayTimeoutSec: num(p.away_timeout_sec ?? p.awayTimeoutSec, base.presence.awayTimeoutSec),
      speechThreshold: num(p.speech_threshold ?? p.speechThreshold, base.presence.speechThreshold),
      ambientEnergyFloor: num(
        p.ambient_energy_floor ?? p.ambientEnergyFloor,
        base.presence.ambientEnergyFloor,
      ),
      ownerPresenceTtlSec: num(
        p.owner_presence_ttl_sec ?? p.ownerPresenceTtlSec,
        base.presence.ownerPresenceTtlSec,
      ),
    }
  }

  return base
}

function parseClientBlock(data: Record<string, unknown>): Pick<ClientConfig, 'lowPowerMode' | 'eventLoopProbeMs'> {
  const c = (data.client ?? {}) as Record<string, unknown>
  const lowPower = !!(c.low_power_mode ?? c.lowPowerMode)
  const probeMs = num(c.event_loop_probe_ms ?? c.eventLoopProbeMs, 1000)
  return {
    lowPowerMode: lowPower,
    eventLoopProbeMs: probeMs,
  }
}

function parseCompanionBlock(data: Record<string, unknown>): CompanionPresenceConfig {
  const c = (data.companion ?? {}) as Record<string, unknown>
  const base = { ...DEFAULT_COMPANION_PRESENCE }
  return {
    enabled: c.presence_chat_enabled !== false,
    intervalSec: num(c.presence_chat_interval_sec, base.intervalSec),
    cooldownMin: num(c.presence_chat_cooldown_min, base.cooldownMin),
    dailyMax: num(c.presence_chat_daily_max, base.dailyMax),
  }
}

function applyPublicConfig(base: string, data: Record<string, unknown>) {
  if (typeof data.api_base === 'string' && data.api_base) {
    setApiBase(data.api_base)
  } else {
    setApiBase(base)
  }
  _clientConfig = {
    apiBase: _apiBase,
    realtimeEnabled: data.realtime_enabled !== false,
    writeApproval: !!data.write_approval,
    growthEnabled: data.growth_enabled !== false,
    visionEnabled: !!data.vision_enabled,
    ...parseVisionBlock(data),
    ...parseClientBlock(data),
    realtime: parseRealtimeBlock(data.realtime),
    companionPresence: parseCompanionBlock(data),
  }
  // 服务端 vision 开关同步到客户端 localStorage
  if (typeof window !== 'undefined') {
    void import('@/services/visionCapture').then(({ ensureVisionCaptureDefault }) => {
      ensureVisionCaptureDefault()
    })
  }
  return _clientConfig
}

/** 运行时从服务端 GET /api/v1/public/config 拉取客户端配置（不读本地 yaml）。 */
export async function initClientConfig(): Promise<ClientConfig> {
  if (typeof window !== 'undefined' && (window.location.port === '1420' || window.location.port === '1421')) {
    setApiBase('')
    try {
      const res = await fetch('/api/v1/public/config')
      if (res.ok) {
        return applyPublicConfig('', (await res.json()) as Record<string, unknown>)
      }
    } catch {
      // proxy may be down; keep defaults
    }
    _clientConfig = { ..._clientConfig, apiBase: '' }
    return _clientConfig
  }

  const fallbacks = ['http://localhost:8081', 'http://localhost:8080']
  for (const base of fallbacks) {
    try {
      const res = await fetch(`${base}/api/v1/public/config`)
      if (res.ok) {
        return applyPublicConfig(base, (await res.json()) as Record<string, unknown>)
      }
    } catch {
      // try next
    }
  }

  setApiBase('http://localhost:8081')
  return _clientConfig
}

export function resolveSttMode(
  cfg: RealtimeClientConfig,
  localSupported: boolean,
): 'cloud' | 'local' {
  if (cfg.sttMode === 'local') return localSupported ? 'local' : 'cloud'
  if (cfg.sttMode === 'auto' && localSupported) {
    return 'local'
  }
  return 'cloud'
}

export function resolveTtsMode(
  cfg: RealtimeClientConfig,
  localSupported: boolean,
): 'cloud' | 'local' {
  if (cfg.ttsMode === 'local') return localSupported ? 'local' : 'cloud'
  if (cfg.ttsMode === 'auto' && localSupported) {
    return 'local'
  }
  return 'cloud'
}

/** 用户设置里的 stt_mode 覆盖 public config（设置页保存到 user preferences）。 */
export function withUserSttMode(
  cfg: RealtimeClientConfig,
  mode: string | undefined | null,
): RealtimeClientConfig {
  if (mode === 'cloud' || mode === 'local' || mode === 'auto') {
    return { ...cfg, sttMode: mode }
  }
  return cfg
}

export function withUserTtsMode(
  cfg: RealtimeClientConfig,
  mode: string | undefined | null,
): RealtimeClientConfig {
  if (mode === 'cloud' || mode === 'local' || mode === 'auto') {
    return { ...cfg, ttsMode: mode }
  }
  return cfg
}

export function readCachedTtsMode(): TtsMode | null {
  if (typeof window === 'undefined') return null
  const v = localStorage.getItem('mochi_tts_mode')
  if (v === 'cloud' || v === 'local' || v === 'auto') return v
  return null
}

export function readCachedSttMode(): SttMode | null {
  if (typeof window === 'undefined') return null
  const v = localStorage.getItem('mochi_stt_mode')
  if (v === 'cloud' || v === 'local' || v === 'auto') return v
  return null
}
