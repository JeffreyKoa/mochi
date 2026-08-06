import { isTauri } from '@/services/chatWindow'

/** 与 PCMCapture 一致的采集约束（warm-up 与正式采集须相同，避免权限状态不一致） */
export const MIC_CAPTURE_CONSTRAINTS: MediaStreamConstraints = {
  audio: {
    echoCancellation: true,
    noiseSuppression: true,
    autoGainControl: true,
  },
}

/** 麦克风被拒绝时的用户提示（Tauri 桌宠 vs 浏览器 dev） */
export function micPermissionDeniedMessage(): string {
  if (isTauri()) {
    return (
      '麦克风被拒绝。请打开：Windows 设置 → 隐私 → 麦克风，' +
      '开启「桌面应用」访问；若仍不行，在设置里点「修复麦克风权限」后重试。'
    )
  }
  return '麦克风被拒绝。请点击地址栏左侧锁图标 → 网站设置 → 麦克风 → 允许，然后刷新页面。'
}

/** 调用 Tauri 重置 WebView2 麦克风站点权限（曾点「阻止」时） */
export async function resetTauriMicrophonePermission(): Promise<boolean> {
  if (!isTauri()) return false
  try {
    const { invoke } = await import('@tauri-apps/api/core')
    await invoke('reset_microphone_permission')
    return true
  } catch (e) {
    console.warn('[mic] reset_microphone_permission failed', e)
    return false
  }
}

/** 重新预授权 WebView2 麦克风（Rust 侧 SetPermissionState + PermissionRequested） */
export async function ensureTauriMicrophonePermission(): Promise<boolean> {
  if (!isTauri()) return false
  try {
    const { invoke } = await import('@tauri-apps/api/core')
    await invoke('ensure_microphone_permission')
    return true
  } catch (e) {
    console.warn('[mic] ensure_microphone_permission failed', e)
    return false
  }
}

/** Tauri：Rust 预授权后再 getUserMedia 验证麦克风是否可用 */
export async function ensureTauriMicrophoneAccess(): Promise<boolean> {
  if (!isTauri()) return warmUpMicrophoneAccess()
  await ensureTauriMicrophonePermission()
  if (await warmUpMicrophoneAccess()) return true
  // WebView2 曾点「阻止」：重置站点权限后再试一次
  const resetOk = await resetTauriMicrophonePermission()
  if (!resetOk) return false
  await ensureTauriMicrophonePermission()
  return warmUpMicrophoneAccess()
}

/** 启动时预申请一次麦克风（失败不阻塞，仅日志） */
export async function warmUpMicrophoneAccess(): Promise<boolean> {
  if (!navigator.mediaDevices?.getUserMedia) return false
  try {
    const stream = await navigator.mediaDevices.getUserMedia(MIC_CAPTURE_CONSTRAINTS)
    stream.getTracks().forEach((t) => t.stop())
    return true
  } catch (e) {
    console.warn('[mic] getUserMedia failed', e)
    return false
  }
}
