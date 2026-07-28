import { emit, listen, type UnlistenFn } from '@tauri-apps/api/event'
import { WebviewWindow } from '@tauri-apps/api/webviewWindow'
import { isTauri } from '@/services/chatWindow'

/** Which Tauri window owns the single /ws/voice connection. */
export type VoiceOwner = 'pet' | 'chat'

const STORAGE_KEY = 'mochi:voice-owner'
export const VOICE_OWNER_EVENT = 'voice-session-owner'

export function getStoredVoiceOwner(): VoiceOwner | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === 'chat' || v === 'pet' ? v : null
  } catch {
    return null
  }
}

function setStoredVoiceOwner(owner: VoiceOwner | null) {
  try {
    if (owner) localStorage.setItem(STORAGE_KEY, owner)
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
}

async function findChatWindow(): Promise<WebviewWindow | null> {
  try {
    return (await WebviewWindow.getByLabel('chat')) ?? null
  } catch {
    return null
  }
}

/** True when the Tauri chat popup exists and is visible. */
export async function isChatWindowVisible(): Promise<boolean> {
  if (!isTauri()) return false
  const chat = await findChatWindow()
  if (!chat) return false
  try {
    return await chat.isVisible()
  } catch {
    return false
  }
}

async function broadcastVoiceOwner(owner: VoiceOwner) {
  setStoredVoiceOwner(owner)
  if (!isTauri()) return
  try {
    await emit(VOICE_OWNER_EVENT, { owner })
  } catch {
    // optional
  }
}

/** Take voice ownership (the other window should disconnect on event). */
export async function claimVoiceOwner(owner: VoiceOwner) {
  await broadcastVoiceOwner(owner)
}

/** Release ownership; hand voice back to pet when chat closes. */
export async function releaseVoiceOwner(owner: VoiceOwner) {
  if (getStoredVoiceOwner() !== owner) return
  await broadcastVoiceOwner('pet')
}

export interface VoiceOwnerHandlers {
  onAcquire: () => void | Promise<void>
  onYield: () => void | Promise<void>
}

/** Listen for voice ownership changes in this webview. */
export async function setupVoiceOwnerListener(
  self: VoiceOwner,
  handlers: VoiceOwnerHandlers,
): Promise<UnlistenFn | null> {
  if (!isTauri()) return null

  const handle = async (owner: VoiceOwner) => {
    if (owner === self) await handlers.onAcquire()
    else await handlers.onYield()
  }

  const stored = getStoredVoiceOwner()
  if (stored && stored !== self) {
    await handlers.onYield()
  } else if (stored === self) {
    await handlers.onAcquire()
  }

  return listen<{ owner?: VoiceOwner }>(VOICE_OWNER_EVENT, (event) => {
    const owner = event.payload?.owner
    if (owner === 'pet' || owner === 'chat') void handle(owner)
  })
}

export async function currentWindowLabel(): Promise<string> {
  if (!isTauri()) return 'browser'
  try {
    const { getCurrentWindow } = await import('@tauri-apps/api/window')
    return getCurrentWindow().label
  } catch {
    return 'browser'
  }
}

/** Popup chat window (label=chat), not inline docked panel in pet shell. */
export async function isPopupChatWindow(): Promise<boolean> {
  return (await currentWindowLabel()) === 'chat'
}
