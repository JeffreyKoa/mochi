/** X-TTS Matcha sidecar HTTP 客户端（本地 TTS POC）。 */

export interface XTtsHealth {
  status: string
  engine?: string
  sample_rate?: number
  version?: string
}

/** 与 sidecar 内置版本对齐；不匹配时 DEV 告警。 */
export const EXPECTED_XTTS_VERSION = '1'

const DEFAULT_TIMEOUT_MS = 15000
const PROBE_TIMEOUT_MS = 2500

function normalizeBaseUrl(url: string): string {
  return url.replace(/\/+$/, '')
}

/**
 * 解析实际 fetch 地址。
 * Dev（Vite 1420）：走同源 /x-tts 代理，绕过浏览器 CORS。
 * 生产 / sidecar 已带 CORS：直连 127.0.0.1:8767。
 */
export function resolveXTtsFetchBase(configUrl: string): string {
  const base = normalizeBaseUrl(configUrl)
  if (!base) return base
  if (import.meta.env.DEV && typeof window !== 'undefined') {
    try {
      const u = new URL(base)
      const local =
        u.hostname === '127.0.0.1' ||
        u.hostname === 'localhost' ||
        u.port === '8767'
      if (local) return '/x-tts'
    } catch {
      // 相对路径等：直接使用
    }
  }
  return base
}

/** 探测 sidecar 是否在线。 */
export async function probeXTtsReachable(
  baseUrl: string,
  timeoutMs = PROBE_TIMEOUT_MS,
): Promise<boolean> {
  const base = resolveXTtsFetchBase(baseUrl)
  if (!base) return false
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    const res = await fetch(`${base}/health`, { signal: ctrl.signal })
    if (!res.ok) return false
    try {
      const health = (await res.json()) as XTtsHealth
      if (health.version && health.version !== EXPECTED_XTTS_VERSION && import.meta.env.DEV) {
        console.warn(
          '[x-tts] sidecar version mismatch: got %s expected %s',
          health.version,
          EXPECTED_XTTS_VERSION,
        )
      }
    } catch {
      // health JSON 解析失败仍视为 reachable
    }
    return true
  } catch {
    return false
  } finally {
    clearTimeout(timer)
  }
}

/** 读取 health JSON（DEV 诊断用）。 */
export async function fetchXTtsHealth(baseUrl: string): Promise<XTtsHealth | null> {
  const base = resolveXTtsFetchBase(baseUrl)
  if (!base) return null
  try {
    const res = await fetch(`${base}/health`)
    if (!res.ok) return null
    return (await res.json()) as XTtsHealth
  } catch {
    return null
  }
}

/** 合成语音；默认返回 WAV ArrayBuffer。 */
export async function synthesizeXTts(
  baseUrl: string,
  text: string,
  options?: { speed?: number; pcm?: boolean; timeoutMs?: number; signal?: AbortSignal },
): Promise<ArrayBuffer | null> {
  const trimmed = text.trim()
  if (!trimmed) return null

  const base = resolveXTtsFetchBase(baseUrl)
  const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  // 外部 signal（barge-in）与超时合并
  const onExternalAbort = () => ctrl.abort()
  if (options?.signal) {
    if (options.signal.aborted) {
      clearTimeout(timer)
      return null
    }
    options.signal.addEventListener('abort', onExternalAbort, { once: true })
  }

  try {
    const res = await fetch(`${base}/synthesize`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json; charset=utf-8' },
      body: JSON.stringify({
        text: trimmed,
        speed: options?.speed ?? 1.0,
        pcm: options?.pcm ?? false,
      }),
      signal: ctrl.signal,
    })
    if (!res.ok) {
      if (import.meta.env.DEV) {
        console.warn('[x-tts] synthesize failed', res.status, await res.text().catch(() => ''))
      }
      return null
    }
    return await res.arrayBuffer()
  } catch (e) {
    if (import.meta.env.DEV) console.warn('[x-tts] synthesize error', e)
    return null
  } finally {
    clearTimeout(timer)
    options?.signal?.removeEventListener('abort', onExternalAbort)
  }
}
