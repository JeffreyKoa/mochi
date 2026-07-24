import { usePetStore } from '@/stores/petStore'
import { mapServerAnimation } from '@/utils/animation'
import { broadcastProactive, notifyTasksRefresh, type ProactivePayload } from './proactiveSync'

let lastShown = { text: '', at: 0 }

/** Show reminder/proactive UI. Caller should append chat message separately. */
export function handleProactiveMessage(payload: ProactivePayload) {
  if (!payload.message?.trim()) return
  const now = Date.now()
  if (payload.message === lastShown.text && now-lastShown.at < 8000) return
  lastShown = { text: payload.message, at: now }
  const pet = usePetStore()
  pet.setAnimation(mapServerAnimation(payload.animation ?? 'happy'))
  pet.showSpeechBubble(payload.message, 12000)
  speakReminder(payload.message)
  void broadcastProactive(payload)
  void notifyTasksRefresh()
}

function speakReminder(text: string) {
  if (typeof window === 'undefined' || !window.speechSynthesis) return
  window.speechSynthesis.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'zh-CN'
  utterance.rate = 1.05
  window.speechSynthesis.speak(utterance)
}
