import { LogicalSize, PhysicalPosition, PhysicalSize } from '@tauri-apps/api/dpi'
import { emit } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { WebviewWindow } from '@tauri-apps/api/webviewWindow'

export const PET_W = 200
export const PET_H = 220
export const CHAT_W = 320
export const CHAT_H = 440
export const CHAT_GAP = 8
export const PET_WITH_CHAT_W = PET_W + CHAT_GAP + CHAT_W
export const PET_WITH_CHAT_H = Math.max(PET_H, CHAT_H)
export const LOGIN_W = 360
export const LOGIN_H = 420
export const SIDE_PANEL_W = 320
export const SIDE_PANEL_H = 440
export const PET_WITH_SIDE_W = PET_W + CHAT_GAP + SIDE_PANEL_W
export const PET_WITH_SIDE_H = Math.max(PET_H, SIDE_PANEL_H)

let tauriCached: boolean | null = null

export function isTauri(): boolean {
  if (tauriCached != null) return tauriCached
  if (import.meta.env.TAURI_ENV_PLATFORM != null) {
    tauriCached = true
    return true
  }
  if (typeof window !== 'undefined') {
    const w = window as Window & { __TAURI_INTERNALS__?: unknown; __TAURI__?: unknown }
    tauriCached = w.__TAURI_INTERNALS__ != null || w.__TAURI__ != null
    return tauriCached
  }
  tauriCached = false
  return false
}

export function isPetWindowLabel(label: string): boolean {
  return label === 'pet' || label === 'main'
}

async function invokeCmd(cmd: string, args: Record<string, unknown> = {}): Promise<boolean> {
  try {
    const { invoke } = await import('@tauri-apps/api/core')
    await invoke(cmd, args)
    return true
  } catch (e) {
    console.warn(`[window] ${cmd}`, e)
    return false
  }
}

async function invokeWithLabel(cmd: string): Promise<boolean> {
  return invokeCmd(cmd, { label: getCurrentWindow().label })
}

function windowExpanded(width: number, height: number, beforeW: number): boolean {
  return width >= PET_WITH_CHAT_W - 80 && height >= PET_WITH_CHAT_H - 80
    || width >= beforeW + 200
}

async function findChatWindow(): Promise<WebviewWindow | null> {
  try {
    const chat = await WebviewWindow.getByLabel('chat')
    if (chat) return chat
  } catch {
    // continue
  }

  try {
    const { getAllWindows } = await import('@tauri-apps/api/window')
    const labels = await getAllWindows()
    if (labels.includes('chat')) {
      return await WebviewWindow.getByLabel('chat')
    }
  } catch {
    // continue
  }

  return null
}

async function placeChatBesidePet(chat: WebviewWindow): Promise<void> {
  const petWin = getCurrentWindow()
  const pos = await petWin.outerPosition()
  const petSize = await petWin.outerSize()
  const chatSize = await chat.outerSize()

  let x = pos.x + petSize.width + CHAT_GAP
  let y = pos.y

  try {
    const { currentMonitor } = await import('@tauri-apps/api/window')
    const mon = await currentMonitor()
    if (mon) {
      const right = mon.position.x + mon.size.width
      const bottom = mon.position.y + mon.size.height
      if (x + chatSize.width > right - 8) {
        x = pos.x - chatSize.width - CHAT_GAP
      }
      x = Math.max(mon.position.x + 4, Math.min(x, right - chatSize.width - 4))
      y = Math.max(mon.position.y + 4, Math.min(y, bottom - chatSize.height - 4))
    }
  } catch {
    // use default x/y
  }

  await chat.setPosition(new PhysicalPosition(Math.round(x), Math.round(y)))
}

/** Keep popup chat window beside pet after manual drag. */
export async function syncChatPopupPosition(): Promise<void> {
  if (!isTauri()) return
  try {
    const chat = await findChatWindow()
    if (!chat) return
    const visible = await chat.isVisible()
    if (!visible) return
    await placeChatBesidePet(chat)
  } catch {
    // optional
  }
}

async function ensureChatWebview(): Promise<WebviewWindow | null> {
  const existing = await findChatWindow()
  if (existing) return existing

  try {
    const chat = new WebviewWindow('chat', {
      url: '/',
      width: CHAT_W,
      height: CHAT_H,
      decorations: false,
      alwaysOnTop: true,
      transparent: false,
      resizable: false,
      skipTaskbar: false,
      focus: true,
    })

    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('chat window create timeout')), 8000)
      chat.once('tauri://created', () => {
        clearTimeout(timer)
        resolve()
      })
      chat.once('tauri://error', (e) => {
        clearTimeout(timer)
        reject(e)
      })
    })

    return chat
  } catch (e) {
    console.warn('[chat] ensure webview', e)
    return await findChatWindow()
  }
}

async function revealChatWindow(chat: WebviewWindow): Promise<boolean> {
  try {
    await placeChatBesidePet(chat)
    await chat.show()
    await chat.setFocus()
    try {
      await emit('chat-opened', { token: localStorage.getItem('mochi_token') })
    } catch {
      // optional
    }
    return true
  } catch (e) {
    console.warn('[chat] reveal', e)
    return false
  }
}

/** Show separate chat window to the right of pet. */
export async function showChatPopupWindow(): Promise<boolean> {
  if (!isTauri()) return false

  const existing = await findChatWindow()
  if (existing && (await revealChatWindow(existing))) return true

  if (await invokeWithLabel('show_chat_window')) {
    try {
      await emit('chat-opened', { token: localStorage.getItem('mochi_token') })
    } catch {
      // optional
    }
    return true
  }

  const chat = await ensureChatWebview()
  if (!chat) return false
  return revealChatWindow(chat)
}

/** Expand pet window: pet left, chat panel right. */
export async function openChatPanel(): Promise<boolean> {
  if (!isTauri()) return true

  try {
    const win = getCurrentWindow()
    if (!isPetWindowLabel(win.label)) return false

    const sizeBefore = await win.outerSize()

    try {
      await win.setResizable(true)
    } catch {
      // optional
    }

    for (const size of [
      new LogicalSize(PET_WITH_CHAT_W, PET_WITH_CHAT_H),
      new PhysicalSize(PET_WITH_CHAT_W, PET_WITH_CHAT_H),
    ]) {
      try {
        await win.setSize(size)
        const outer = await win.outerSize()
        if (windowExpanded(outer.width, outer.height, sizeBefore.width)) return true
      } catch (e) {
        console.warn('[chat] setSize', e)
      }
    }

    await invokeWithLabel('expand_pet_for_chat')

    const outer = await win.outerSize()
    return windowExpanded(outer.width, outer.height, sizeBefore.width)
  } catch (e) {
    console.warn('[chat] openChatPanel', e)
    return false
  }
}

export async function hideChatPopupOnly(): Promise<void> {
  if (!isTauri()) return
  await invokeCmd('hide_chat_window')
  try {
    const chat = await findChatWindow()
    if (chat) await chat.hide()
  } catch {
    // ignore
  }
}

export async function closeChatPanel(): Promise<void> {
  await hideChatPopupOnly()
  if (!isTauri()) return
  await invokeWithLabel('collapse_pet_chat')
  try {
    const win = getCurrentWindow()
    if (isPetWindowLabel(win.label)) {
      await win.setSize(new LogicalSize(PET_W, PET_H))
    }
  } catch {
    // ignore
  }
}

export async function closeChatPopup(): Promise<void> {
  await closeChatPanel()
  try {
    await emit('chat-closed', {})
  } catch {
    // optional
  }
}

export async function setWindowSize(width: number, height: number) {
  if (!isTauri()) return
  const win = getCurrentWindow()
  try {
    await win.setResizable(true)
    await win.setSize(new LogicalSize(width, height))
  } catch {
    await win.setSize(new PhysicalSize(width, height))
  }
}

export async function setPetOnlyLayout() {
  if (!isTauri()) return
  await invokeWithLabel('collapse_pet_chat')
  await setWindowSize(PET_W, PET_H)
}

export async function setLoginLayout() {
  await setWindowSize(LOGIN_W, LOGIN_H)
}

export async function ensurePetWindowVisible() {
  if (!isTauri()) return
  const win = getCurrentWindow()
  if (!isPetWindowLabel(win.label)) return

  try {
    const visible = await win.isVisible()
    if (!visible) {
      await win.show()
      await win.setAlwaysOnTop(true)
    }

    const pos = await win.outerPosition()
    const size = await win.outerSize()
    const { availableMonitors, primaryMonitor } = await import('@tauri-apps/api/window')
    const monitors = await availableMonitors()
    const primary = (await primaryMonitor()) ?? monitors[0]
    if (!primary) {
      await win.center()
      return
    }

    const intersectsAny = monitors.some((mon) => {
      const minX = mon.position.x
      const minY = mon.position.y
      const maxX = mon.position.x + mon.size.width
      const maxY = mon.position.y + mon.size.height
      return (
        pos.x + size.width > minX + 8 &&
        pos.y + size.height > minY + 8 &&
        pos.x < maxX - 8 &&
        pos.y < maxY - 8
      )
    })

    if (!intersectsAny) {
      await win.center()
      return
    }

    const mon = (await win.currentMonitor()) ?? primary
    const minX = mon.position.x
    const minY = mon.position.y
    const maxX = mon.position.x + mon.size.width - size.width
    const maxY = mon.position.y + mon.size.height - size.height

    const x = Math.max(minX, Math.min(pos.x, maxX))
    const y = Math.max(minY, Math.min(pos.y, maxY))
    if (x !== pos.x || y !== pos.y) {
      await win.setPosition(new PhysicalPosition(x, y))
    }
  } catch {
    try {
      await win.show()
      await win.center()
    } catch {
      // ignore
    }
  }
}

export async function initPetWindowChrome() {
  if (!isTauri()) return
  const win = getCurrentWindow()
  if (!isPetWindowLabel(win.label)) return
  try {
    await win.setShadow(false)
    await win.show()
    await win.setFocus()
  } catch {
    // optional
  }
}

export const expandPetWindowForChat = openChatPanel
export const collapsePetWindowForChat = closeChatPanel
