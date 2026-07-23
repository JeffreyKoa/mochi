import { emit, listen, type UnlistenFn } from '@tauri-apps/api/event'
import { isTauri } from './chatWindow'

export type ProactivePayload = {
  message: string
  animation?: string
}

const EVENT = 'mochi-proactive'

/** Pet window → chat popup / other webviews */
export async function broadcastProactive(payload: ProactivePayload): Promise<void> {
  if (!isTauri()) return
  try {
    await emit(EVENT, payload)
  } catch {
    // optional cross-window sync
  }
}

export async function listenProactive(
  handler: (payload: ProactivePayload) => void,
): Promise<UnlistenFn> {
  if (!isTauri()) return () => {}
  try {
    return await listen<ProactivePayload>(EVENT, (event) => {
      if (event.payload?.message) handler(event.payload)
    })
  } catch {
    return () => {}
  }
}
