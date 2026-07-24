let _apiBase = ''

export interface RealtimeVadConfig {
  silenceMs: number
  minSpeechMs: number
  endpointingEnabled: boolean
}

export interface RealtimeBargeInConfig {
  echoGuardMs: number
  peakThreshold: number
  bargeInMs: number
}

export type SttMode = 'cloud' | 'local' | 'auto'

export interface RealtimeClientConfig {
  sttMode: SttMode
  speechLocale: string
  vad: RealtimeVadConfig
  bargeIn: RealtimeBargeInConfig
}

export interface ClientConfig {
  apiBase: string
  realtimeEnabled: boolean
  writeApproval: boolean
  growthEnabled: boolean
  realtime: RealtimeClientConfig
}

export const DEFAULT_REALTIME: RealtimeClientConfig = {
  sttMode: 'auto',
  speechLocale: 'zh-CN',
  vad: {
    silenceMs: 700,
    minSpeechMs: 300,
    endpointingEnabled: true,
  },
  bargeIn: {
    echoGuardMs: 1800,
    peakThreshold: 0.06,
    bargeInMs: 800,
  },
}

let _clientConfig: ClientConfig = {
  apiBase: '',
  realtimeEnabled: true,
  writeApproval: false,
  growthEnabled: true,
  realtime: { ...DEFAULT_REALTIME },
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

export function setApiBase(url: string) {
  _apiBase = url.replace(/\/$/, '')
  _clientConfig = { ..._clientConfig, apiBase: _apiBase }
}

function num(v: unknown, fallback: number): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback
}

function parseRealtimeBlock(raw: unknown): RealtimeClientConfig {
  const base = { ...DEFAULT_REALTIME, vad: { ...DEFAULT_REALTIME.vad }, bargeIn: { ...DEFAULT_REALTIME.bargeIn } }
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
    realtime: parseRealtimeBlock(data.realtime),
  }
  return _clientConfig
}

/** 初始化 API 地址：Vite 开发走代理（相对路径），Tauri/生产读 config.yaml */
export async function initClientConfig(): Promise<ClientConfig> {
  if (typeof window !== 'undefined' && window.location.port === '1420') {
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
  if (cfg.sttMode === 'cloud') return 'cloud'
  if (cfg.sttMode === 'local') return localSupported ? 'local' : 'cloud'
  return localSupported ? 'local' : 'cloud'
}
