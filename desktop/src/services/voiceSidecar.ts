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

/** 重启本地语音服务（设置页 / 诊断用）。 */
export async function restartVoiceSidecars(): Promise<VoiceSidecarStatus | null> {
  if (!isTauri()) return null
  try {
    return await invoke<VoiceSidecarStatus>('restart_voice_sidecars')
  } catch (e) {
    console.warn('[voice-sidecar] restart failed', e)
    return null
  }
}

/** 等待 sidecar 端口就绪（Release 冷启动模型加载较慢）。 */
export async function waitForVoiceSidecarsReady(opts?: {
  timeoutMs?: number
}): Promise<boolean> {
  if (!isTauri()) return true
  const timeoutMs = opts?.timeoutMs ?? 90_000
  const deadline = Date.now() + timeoutMs
  const cfg = getRealtimeConfig()
  const { probeXAsrServer } = await import('@/services/xAsrClient')

  while (Date.now() < deadline) {
    const st = await getVoiceSidecarStatus()
    if (st?.xasr.state === 'error' || st?.xasr.state === 'skipped') {
      console.warn('[voice-sidecar] x-asr unavailable:', st?.xasr.message)
      return false
    }
    if (await probeXAsrServer(cfg.xasr.wsUrl, 4000)) {
      return true
    }
    await new Promise((r) => setTimeout(r, 1500))
  }
  console.warn('[voice-sidecar] x-asr not ready within timeout')
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
    if (ok) console.info('[voice-sidecar] x-asr port ready')
  })
  return st
}
