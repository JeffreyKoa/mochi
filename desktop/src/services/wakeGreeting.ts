/** 唤醒即时反馈：本地 speechSynthesis，目标 ≤300ms，不依赖 WS/TTS 链路。 */
const WAKE_GREETING_TEXT = '在呢，主人！'

let lastPlayedAt = 0
const MIN_REPEAT_MS = 2000

/** 是否允许唤醒语音（与提醒 TTS 共用开关）。 */
function wakeVoiceEnabled(): boolean {
  if (typeof localStorage === 'undefined') return true
  return localStorage.getItem('mochi_reminder_voice') !== '0'
}

/**
 * 播放唤醒问候语。重复点击在 MIN_REPEAT_MS 内去重，避免叠音。
 * @returns 是否实际触发了 speak
 */
export function playWakeGreeting(text = WAKE_GREETING_TEXT): boolean {
  if (typeof window === 'undefined' || !window.speechSynthesis) return false
  if (!wakeVoiceEnabled()) return false

  const now = Date.now()
  if (now - lastPlayedAt < MIN_REPEAT_MS) return false
  lastPlayedAt = now

  // 取消 proactive 等在途 utterance，优先唤醒反馈
  window.speechSynthesis.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'zh-CN'
  utterance.rate = 1.08
  utterance.pitch = 1.05
  window.speechSynthesis.speak(utterance)
  return true
}

export function getWakeGreetingText(): string {
  return WAKE_GREETING_TEXT
}
