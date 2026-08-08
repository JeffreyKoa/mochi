/** Tauri 托管的 X-ASR / X-TTS sidecar 状态（Release 含内置 Python + 模型）。 */

import { invoke } from '@tauri-apps/api/core'
import { getRealtimeConfig } from '@/config'
import { isTauri } from '@/services/chatWindow'

export type SidecarState =
  | 'stopped'
  | 'starting'
  | 'running'
  | 'skipped'
  | 'error'
  | 'external'

export interface SidecarServiceStatus {
  state: SidecarState
  managed: boolean
  message?: string
}

export interface VoiceSidecarStatus {
  managed: boolean
  bundleMode: 'dev' | 'release'
  xasr: SidecarServiceStatus
  xtts: SidecarServiceStatus
}

/** 查询 Rust 侧 sidecar 托管状态。 */
export async function getVoiceSidecarStatus(): Promise<VoiceSidecarStatus | null> {
  if (!isTauri()) return null
  try {
    return await invoke<VoiceSidecarStatus>('get_voice_sidecar_status')
  } catch (e) {
    console.warn('[voice-sidecar] get status failed', e)
    return null
  }
}

/** 同时重启 X-ASR + X-TTS（诊断等批量场景用）。 */
export async function restartVoiceSidecars(): Promise<VoiceSidecarStatus | null> {
  if (!isTauri()) return null
  try {
    return await invoke<VoiceSidecarStatus>('restart_voice_sidecars')
  } catch (e) {
    console.warn('[voice-sidecar] restart all failed', e)
    return null
  }
}

/** 仅重启 X-ASR 语音识别 sidecar。 */
export async function restartXAsrSidecar(): Promise<VoiceSidecarStatus | null> {
  if (!isTauri()) return null
  try {
    return await invoke<VoiceSidecarStatus>('restart_xasr_sidecar')
  } catch (e) {
    console.warn('[voice-sidecar] restart x-asr failed', e)
    return null
  }
}

/** 仅重启 X-TTS 语音合成 sidecar。 */
export async function restartXTtsSidecar(): Promise<VoiceSidecarStatus | null> {
  if (!isTauri()) return null
  try {
    return await invoke<VoiceSidecarStatus>('restart_xtts_sidecar')
  } catch (e) {
    console.warn('[voice-sidecar] restart x-tts failed', e)
    return null
  }
}

/** sidecar 日志目录（Windows：%LOCALAPPDATA%\\Mochi\\logs）。 */
export async function getVoiceLogDir(): Promise<string | null> {
  if (!isTauri()) return null
  try {
    return await invoke<string>('get_voice_log_dir')
  } catch {
    return null
  }
}

/** 用资源管理器打开 sidecar 日志目录。 */
export async function openVoiceLogs(): Promise<boolean> {
  if (!isTauri()) return false
  try {
    await invoke('open_voice_logs')
    return true
  } catch (e) {
    console.warn('[voice-sidecar] open logs failed', e)
    return false
  }
}

/** 等待 sidecar 端口就绪（Release 冷启动模型加载较慢）。 */
export async function waitForVoiceSidecarsReady(opts?: {
  timeoutMs?: number
  /** 是否同时等待 X-TTS（默认 false：cloud TTS 由服务端 sidecar 负责）。 */
  requireXtts?: boolean
}): Promise<boolean> {
  if (!isTauri()) return true
  const timeoutMs = opts?.timeoutMs ?? 90_000
  const deadline = Date.now() + timeoutMs
  const cfg = getRealtimeConfig()
  const { probeXAsrServer } = await import('@/services/xAsrClient')
  const { probeXTtsReachable } = await import('@/services/xTtsClient')
  const requireXtts = opts?.requireXtts ?? false

  while (Date.now() < deadline) {
    const st = await getVoiceSidecarStatus()
    if (st?.xasr.state === 'error' || st?.xasr.state === 'skipped') {
      console.warn('[voice-sidecar] x-asr unavailable:', st?.xasr.message)
      return false
    }
    if (requireXtts && st?.xtts.state === 'error') {
      console.warn('[voice-sidecar] x-tts unavailable:', st?.xtts.message)
      return false
    }
    const xasrOk = await probeXAsrServer(cfg.xasr.wsUrl, 4000)
    const xttsOk = !requireXtts || (await probeXTtsReachable(cfg.xtts.baseUrl, 4000))
    if (xasrOk && xttsOk) {
      return true
    }
    await new Promise((r) => setTimeout(r, 1500))
  }
  console.warn('[voice-sidecar] sidecar not ready within timeout (xasr/xtts)')
  return false
}

/** 仅等待 X-TTS HTTP sidecar（文字聊天 / 纯 TTS 场景）。 */
export async function waitForXTtsSidecarReady(opts?: {
  timeoutMs?: number
}): Promise<boolean> {
  const cfg = getRealtimeConfig()
  const timeoutMs = opts?.timeoutMs ?? 60_000
  const deadline = Date.now() + timeoutMs
  const { probeXTtsReachable } = await import('@/services/xTtsClient')

  while (Date.now() < deadline) {
    if (isTauri()) {
      const st = await getVoiceSidecarStatus()
      if (st?.xtts.state === 'error') {
        console.warn('[voice-sidecar] x-tts unavailable:', st?.xtts.message)
        return false
      }
    }
    if (await probeXTtsReachable(cfg.xtts.baseUrl, 4000)) {
      return true
    }
    await new Promise((r) => setTimeout(r, 1500))
  }
  console.warn('[voice-sidecar] x-tts not ready within timeout')
  return false
}

/** App 启动时记录 sidecar 托管信息（sidecar 已在 Rust setup 拉起）。 */
export async function bootstrapVoiceSidecars(): Promise<VoiceSidecarStatus | null> {
  const st = await getVoiceSidecarStatus()
  if (st) {
    console.info(
      `[voice-sidecar] mode=${st.bundleMode} xasr=${st.xasr.state} xtts=${st.xtts.state}`,
      st,
    )
  }
  // 后台等待就绪，不阻塞 UI
  void waitForVoiceSidecarsReady({ timeoutMs: 90_000 }).then((ok) => {
    if (ok) console.info('[voice-sidecar] x-asr + x-tts ports ready')
  })
  return st
}
