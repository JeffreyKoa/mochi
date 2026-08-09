import { usePetStore } from '@/stores/petStore'
import { broadcastProactive, notifyTasksRefresh, type ProactivePayload } from './proactiveSync'

let lastShown = { text: '', at: 0 }

export function wasProactiveRecentlyShown(text: string): boolean {
  return text === lastShown.text && Date.now() - lastShown.at < 8000
}

export type ProactiveOptions = {
  /** When true, use persistent reminder bubble (voice chat should pass true). */
  priority?: boolean
}

/** Show reminder/proactive UI. 语音朗读由 realtimeStore.speakCloudOnly 经 /ws/voice 下发。 */
export function handleProactiveMessage(payload: ProactivePayload, opts: ProactiveOptions = {}) {
  if (!payload.message?.trim()) return
  const now = Date.now()
  if (payload.message === lastShown.text && now - lastShown.at < 8000) return
  lastShown = { text: payload.message, at: now }

  const pet = usePetStore()
  pet.setServerAnimation(payload.animation ?? 'happy')
  if (opts.priority) {
    pet.showReminderBubble(payload.message, 15000)
  } else {
    pet.showSpeechBubble(payload.message, 12000)
  }
  void broadcastProactive(payload)
  void notifyTasksRefresh()
}
