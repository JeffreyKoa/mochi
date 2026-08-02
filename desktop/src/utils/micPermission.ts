import { isTauri } from '@/services/chatWindow'

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

/** 启动时预申请一次麦克风（失败不阻塞，仅日志） */
export async function warmUpMicrophoneAccess(): Promise<boolean> {
  if (!navigator.mediaDevices?.getUserMedia) return false
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    stream.getTracks().forEach((t) => t.stop())
    return true
  } catch {
    return false
  }
}
