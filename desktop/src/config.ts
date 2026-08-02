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

export interface RealtimeClientConfig {
  sttMode: SttMode
  speechLocale: string
  vad: RealtimeVadConfig
  bargeIn: RealtimeBargeInConfig
  voiceprint: RealtimeVoiceprintConfig
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
  realtime: RealtimeClientConfig
}

export const DEFAULT_SILERO: RealtimeSileroVadConfig = {
  positiveThreshold: 0.5,
  negativeThreshold: 0.35,
  redemptionMs: 1200,
  minSpeechMs: 350,
  preSpeechPadMs: 300,
}

export const DEFAULT_REALTIME: RealtimeClientConfig = {
  sttMode: 'auto',
  speechLocale: 'zh-CN',
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
  realtime: { ...DEFAULT_REALTIME },
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

export function getPresenceConfig(): RealtimePresenceConfig {
  return _clientConfig.realtime.presence
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
    vad: { ...DEFAULT_REALTIME.vad, silero: { ...DEFAULT_REALTIME.vad.silero } },
    bargeIn: { ...DEFAULT_REALTIME.bargeIn },
    voiceprint: { ...DEFAULT_REALTIME.voiceprint },
    presence: { ...DEFAULT_REALTIME.presence },
  }
  if (!raw || typeof raw !== 'object') return base

  const r = raw as Record<string, unknown>
  const mode = String(r.stt_mode ?? r.sttMode ?? base.sttMode)
  if (mode === 'cloud' || mode === 'local' || mode === 'auto') {
    base.sttMode = mode
  }

  if (typeof r.speech_locale === 'string' && r.speech_locale) {
    base.speechLocale = r.speech_locale
  } else if (typeof r.speechLocale === 'string' && r.speechLocale) {
    base.speechLocale = r.speechLocale
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
    realtime: parseRealtimeBlock(data.realtime),
  }
  // 服务端开启 vision 时，客户端默认 opt-in（用户可在设置里关）
  if (_clientConfig.visionEnabled && typeof window !== 'undefined') {
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
  return 'cloud'
}
