import { emit, listen, type UnlistenFn } from '@tauri-apps/api/event'
import { isTauri } from './chatWindow'

export type ProactivePayload = {
  message: string
  animation?: string
}

const EVENT = 'mochi-proactive'
const TASKS_REFRESH = 'mochi-tasks-refresh'

/** Pet window → chat popup / other webviews */
export async function broadcastProactive(payload: ProactivePayload): Promise<void> {
  if (!isTauri()) return
  try {
    await emit(EVENT, payload)
  } catch {
    // optional cross-window sync
  }
}

/** Notify settings panel to reload reminder/todo lists */
export async function notifyTasksRefresh(): Promise<void> {
  if (isTauri()) {
    try {
      await emit(TASKS_REFRESH, {})
    } catch {
      // optional
    }
    return
  }
  window.dispatchEvent(new CustomEvent(TASKS_REFRESH))
}

export async function listenTasksRefresh(handler: () => void): Promise<UnlistenFn> {
  if (isTauri()) {
    try {
      return await listen(TASKS_REFRESH, () => handler())
    } catch {
      return () => {}
    }
  }
  const fn = () => handler()
  window.addEventListener(TASKS_REFRESH, fn)
  return () => window.removeEventListener(TASKS_REFRESH, fn)
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
