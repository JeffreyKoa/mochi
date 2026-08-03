import { LogicalSize, PhysicalPosition } from '@tauri-apps/api/dpi'
import { emit } from '@tauri-apps/api/event'
import { currentMonitor, getCurrentWindow, type Window as TauriWin } from '@tauri-apps/api/window'
import { WebviewWindow, getAllWebviewWindows } from '@tauri-apps/api/webviewWindow'
import { ref } from 'vue'
import { isPlausibleWindowPosition } from '@/services/petRoaming'

export const PET_W = 280
export const PET_H = 280
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

const PANEL_OFFSET = SIDE_PANEL_W + CHAT_GAP

const PANEL_PENDING_KEY = 'mochi:side-panel-pending'

interface SidePanelPending {
  mode: SidePanelMode
  token: string | null
  at: number
}

function stashSidePanelPending(mode: SidePanelMode, token: string | null) {
  const payload: SidePanelPending = { mode, token, at: Date.now() }
  localStorage.setItem(PANEL_PENDING_KEY, JSON.stringify(payload))
}

export function consumeSidePanelPending(maxAgeMs = 60_000): SidePanelPending | null {
  try {
    const raw = localStorage.getItem(PANEL_PENDING_KEY)
    if (!raw) return null
    localStorage.removeItem(PANEL_PENDING_KEY)
    const payload = JSON.parse(raw) as SidePanelPending
    if (Date.now() - payload.at > maxAgeMs) return null
    return payload
  } catch {
    return null
  }
}

async function sleep(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms))
}
let layoutQueue = Promise.resolve()

function runLayoutTask<T>(task: () => Promise<T>): Promise<T> {
  const next = layoutQueue.then(task)
  layoutQueue = next.then(
    () => undefined,
    () => undefined,
  )
  return next
}

/** Side panel visually on the left of Mochi (flex row-reverse). */
export const sidePanelOnLeft = ref(false)

export type SidePanelMode = 'chat' | 'settings'

let popupFollowsPet = false

export function setPopupChatFollowsPet(follow: boolean) {
  popupFollowsPet = follow
}

async function getWindowScale(win: TauriWin): Promise<number> {
  try {
    const scale = await win.scaleFactor()
    return scale > 0 ? scale : 1
  } catch {
    return 1
  }
}

async function getLogicalOuterSize(win: TauriWin): Promise<{ width: number; height: number }> {
  const [outer, scale] = await Promise.all([win.outerSize(), getWindowScale(win)])
  return { width: outer.width / scale, height: outer.height / scale }
}

function sizeNear(actual: number, expected: number, tolerance = 16): boolean {
  return Math.abs(actual - expected) <= tolerance
}

function panelOffsetPx(scale: number): number {
  return Math.round(PANEL_OFFSET * scale)
}

async function setWindowSizeLogical(win: TauriWin, width: number, height: number) {
  try {
    await win.setResizable(true)
  } catch {
    // optional
  }
  try {
    await win.setSize(new LogicalSize(width, height))
  } catch (e) {
    console.warn('[window] LogicalSize failed, retry', e)
    await win.setSize(new LogicalSize(width, height))
  }
}

/** Decide panel side from compact pet position. */
async function resolveSidePanelOnLeft(win: TauriWin): Promise<boolean> {
  const [pos, scale] = await Promise.all([win.outerPosition(), getWindowScale(win)])
  const expandedPx = Math.round(PET_WITH_SIDE_W * scale)
  const offsetPx = panelOffsetPx(scale)
  const margin = 8
  const petScreenX = pos.x

  const mon = await currentMonitor()
  if (!mon) return false

  const monLeft = mon.position.x
  const monRight = mon.position.x + mon.size.width
  const fitsRight = petScreenX + expandedPx <= monRight - margin
  const fitsLeft = petScreenX - offsetPx >= monLeft + margin

  return !fitsRight && fitsLeft
}

let tauriCached: boolean | null = null

type TauriInternals = {
  metadata?: {
    currentWindow?: {
      label?: string
    }
  }
  invoke?: (...args: unknown[]) => unknown
}

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

/** Tauri 注入完成且 currentWindow 可用（避免 metadata 未就绪时 getCurrentWindow 抛错） */
export function isTauriWindowReady(): boolean {
  if (!isTauri()) return false
  const w = window as Window & { __TAURI_INTERNALS__?: TauriInternals }
  return !!w.__TAURI_INTERNALS__?.metadata?.currentWindow?.label
}

/** 安全获取当前 Tauri 窗口；未就绪时返回 null */
export function getTauriWindow(): TauriWin | null {
  if (!isTauriWindowReady()) return null
  try {
    return getCurrentWindow()
  } catch {
    return null
  }
}

/** 读取当前窗口 label（不实例化 Window，避免 metadata 竞态） */
export function readTauriWindowLabel(): string {
  const w = window as Window & { __TAURI_INTERNALS__?: TauriInternals }
  return w.__TAURI_INTERNALS__?.metadata?.currentWindow?.label ?? 'browser'
}

/** 等待 Tauri 注入 currentWindow（启动早期 metadata 可能尚未就绪） */
export async function waitForTauriWindow(maxMs = 3000): Promise<TauriWin | null> {
  const start = Date.now()
  while (Date.now() - start < maxMs) {
    const win = getTauriWindow()
    if (win) return win
    await new Promise((r) => setTimeout(r, 50))
  }
  return null
}

/** 当前 WebView 是否为 pet 窗口且 API 可用 */
export function getPetTauriWindow(): TauriWin | null {
  const win = getTauriWindow()
  if (!win || !isPetWindowLabel(win.label)) return null
  return win
}

/** Tauri invoke 通道是否已注入（早于 metadata 的情况也存在） */
export function isTauriInvokeReady(): boolean {
  if (!isTauri()) return false
  const w = window as Window & { __TAURI_INTERNALS__?: TauriInternals }
  return typeof w.__TAURI_INTERNALS__?.invoke === 'function'
}

export function isPetWindowLabel(label: string): boolean {
  return label === 'pet' || label === 'main'
}

async function invokeCmd(cmd: string, args: Record<string, unknown> = {}): Promise<boolean> {
  if (!isTauriInvokeReady()) return false
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
  const win = getPetTauriWindow()
  if (!win) return false
  return invokeCmd(cmd, { label: win.label })
}

export async function isPetWindowExpanded(): Promise<boolean> {
  if (!isTauriWindowReady()) return false
  const win = getPetTauriWindow()
  if (!win) return false
  const { width, height } = await getLogicalOuterSize(win)
  return sizeNear(width, PET_WITH_CHAT_W) && sizeNear(height, PET_WITH_CHAT_H)
}

async function expandPanelLayoutInner(reposition: boolean): Promise<boolean> {
  const win = getPetTauriWindow()
  if (!win) return false

  if (await isPetWindowExpanded()) {
    if (reposition) await ensurePetWindowVisible()
    return true
  }

  const pos = await win.outerPosition()
  const scale = await getWindowScale(win)
  const onLeft = await resolveSidePanelOnLeft(win)

  await invokeWithLabel('expand_pet_for_chat')
  if (!(await isPetWindowExpanded())) {
    await setWindowSizeLogical(win, PET_WITH_CHAT_W, PET_WITH_CHAT_H)
  }

  sidePanelOnLeft.value = onLeft
  if (onLeft) {
    await win.setPosition(new PhysicalPosition(pos.x - panelOffsetPx(scale), pos.y))
  }

  if (!(await isPetWindowExpanded())) {
    await invokeWithLabel('expand_pet_for_chat')
  }

  if (reposition) await ensurePetWindowVisible()
  return true
}

async function compactPanelLayoutInner(): Promise<void> {
  const win = getPetTauriWindow()
  if (!win) return
  const { width, height } = await getLogicalOuterSize(win)
  if (sizeNear(width, PET_W) && sizeNear(height, PET_H)) {
    sidePanelOnLeft.value = false
    return
  }

  const pos = await win.outerPosition()
  const scale = await getWindowScale(win)
  const petScreenX = sidePanelOnLeft.value ? pos.x + panelOffsetPx(scale) : pos.x

  await invokeWithLabel('collapse_pet_chat')
  await setWindowSizeLogical(win, PET_W, PET_H)
  await win.setPosition(new PhysicalPosition(petScreenX, pos.y))
  sidePanelOnLeft.value = false
}

/** Expand pet shell to fit Mochi + 8px gap + side panel (608×440 logical). */
export async function setExpandedPanelLayout(reposition = false): Promise<boolean> {
  if (!isTauriWindowReady()) return true
  return runLayoutTask(async () => {
    try {
      return await expandPanelLayoutInner(reposition)
    } catch (e) {
      console.warn('[window] expand panel', e)
      return false
    }
  })
}

/** Collapse to pet-only 280×280; keep Mochi screen position. */
export async function setCompactPetLayout(): Promise<void> {
  if (!isTauriWindowReady()) return
  return runLayoutTask(async () => {
    try {
      await compactPanelLayoutInner()
    } catch (e) {
      console.warn('[window] collapse panel', e)
    }
  })
}

/** Sync window size with whether a side panel should be visible. */
export async function syncPanelShellLayout(expanded: boolean, reposition = false): Promise<void> {
  if (!isTauriWindowReady()) return
  if (expanded) {
    await setExpandedPanelLayout(reposition)
  } else {
    await setCompactPetLayout()
  }
}

async function findChatWindow(): Promise<WebviewWindow | null> {
  try {
    const byLabel = (await WebviewWindow.getByLabel('chat')) ?? null
    if (byLabel) return byLabel
    const all = await getAllWebviewWindows()
    return all.find((w) => w.label === 'chat') ?? null
  } catch {
    return null
  }
}

async function ensureSidePanelWindow(): Promise<WebviewWindow | null> {
  const existing = await findChatWindow()
  if (existing) return existing

  try {
    const url = `${window.location.origin}${window.location.pathname}`
    return new WebviewWindow('chat', {
      url,
      title: 'Mochi',
      width: CHAT_W,
      height: CHAT_H,
      transparent: true,
      decorations: false,
      alwaysOnTop: true,
      visible: false,
      resizable: false,
      shadow: false,
    })
  } catch (e) {
    console.warn('[panel] create chat window', e)
    return findChatWindow()
  }
}

async function revealSidePanelWindow(): Promise<boolean> {
  const chat = await ensureSidePanelWindow()
  if (!chat) {
    console.warn('[panel] chat window unavailable')
    return false
  }
  try {
    await placeChatBesidePet(chat)
    await chat.show()
    try {
      await chat.setFocus()
    } catch {
      // focus optional
    }
    return true
  } catch (e) {
    console.warn('[panel] revealSidePanelWindow', e)
    return false
  }
}

async function placeChatBesidePet(
  chat: WebviewWindow,
  overrideX?: number,
  overrideY?: number,
): Promise<void> {
  const petWin = getPetTauriWindow()
  if (!petWin) return
  const scale = await getWindowScale(petWin)
  const pos =
    overrideX != null && overrideY != null
      ? { x: overrideX, y: overrideY }
      : await petWin.outerPosition()
  const gapPx = Math.round(CHAT_GAP * scale)
  const petWPx = Math.round(PET_W * scale)
  const petHPx = Math.round(PET_H * scale)
  const chatWPx = Math.round(CHAT_W * scale)
  const chatHPx = Math.round(CHAT_H * scale)
  const expanded = await isPetWindowExpanded()
  const onLeft = await resolveSidePanelOnLeft(petWin)
  sidePanelOnLeft.value = onLeft

  const petScreenX = expanded && onLeft ? pos.x + panelOffsetPx(scale) : pos.x
  let x = onLeft ? petScreenX - chatWPx - gapPx : petScreenX + petWPx + gapPx
  let y = pos.y + petHPx - chatHPx

  const mon = await currentMonitor()
  if (mon) {
    const right = mon.position.x + mon.size.width
    const bottom = mon.position.y + mon.size.height
    x = Math.max(mon.position.x + 4, Math.min(x, right - chatWPx - 4))
    y = Math.max(mon.position.y + 4, Math.min(y, bottom - chatHPx - 4))
  }

  await chat.setPosition(new PhysicalPosition(Math.round(x), Math.round(y)))
}

async function isChatPopupVisible(): Promise<boolean> {
  try {
    const chat = await findChatWindow()
    if (!chat) return false
    return await chat.isVisible()
  } catch {
    return false
  }
}

export async function syncChatPopupPosition(petX?: number, petY?: number): Promise<void> {
  if (!isTauri()) return
  try {
    const chat = await findChatWindow()
    if (!chat || !(await chat.isVisible())) return
    await placeChatBesidePet(chat, petX, petY)
  } catch {
    // optional
  }
}

export async function syncAttachedSidePanels(petX?: number, petY?: number): Promise<void> {
  // In Tauri, window positioning during movement is handled natively in Rust (WindowEvent::Moved).
  // Avoid sending async IPC setPosition calls to prevent race-condition jitter.
  if (isTauri()) return
  if (!popupFollowsPet) return
  const win = getPetTauriWindow()
  if (!win) return
  if (await isChatPopupVisible()) {
    await syncChatPopupPosition(petX, petY)
  }
}

export async function hideSidePanelPopup(): Promise<void> {
  setPopupChatFollowsPet(false)
  await hideChatPopupOnly()
}

/** Show chat/settings in a separate popup beside the pet (pet stays 280×280). */
export async function showSidePanelPopup(mode: SidePanelMode): Promise<boolean> {
  if (!isTauriWindowReady()) return false
  const win = getPetTauriWindow()
  if (!win) return false

  try {
    void setCompactPetLayout()
    sidePanelOnLeft.value = await resolveSidePanelOnLeft(win)

    const { useAuthStore } = await import('@/stores/authStore')
    const auth = useAuthStore()
    auth.syncFromStorage()
    const token = auth.token ?? null
    stashSidePanelPending(mode, token)

    let shown = await invokeWithLabel('show_chat_window')
    if (!shown) {
      shown = await revealSidePanelWindow()
    }
    if (!shown) return false

    await sleep(80)
    // 统一用 side-panel-opened 传递 mode；chat-opened 仅聊天模式兼容旧监听
    await emit('side-panel-opened', { mode, token })
    if (mode === 'chat') {
      await emit('chat-opened', { mode, token })
    }

    setPopupChatFollowsPet(true)
    return true
  } catch (e) {
    console.warn('[panel] showSidePanelPopup', e)
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

export async function openChatPanel(): Promise<boolean> {
  return showSidePanelPopup('chat')
}

export async function closeChatPanel(collapse = true) {
  await hideSidePanelPopup()
  if (!isTauri() || !collapse) return
  if (!(await isPetWindowExpanded())) return
  return runLayoutTask(async () => {
    try {
      await compactPanelLayoutInner()
    } catch (e) {
      console.warn('[window] collapse panel', e)
    }
  })
}

export async function closeChatPopup(): Promise<void> {
  await closeChatPanel()
  try {
    await emit('chat-closed', {})
  } catch {
    // optional
  }
}

export async function showChatPopupWindow(): Promise<boolean> {
  return showSidePanelPopup('chat')
}

export async function setWindowSize(width: number, height: number) {
  if (!isTauriWindowReady()) return
  const win = getPetTauriWindow()
  if (!win) return
  await setWindowSizeLogical(win, width, height)
}

export async function setPetOnlyLayout() {
  await setCompactPetLayout()
}

export async function setLoginLayout() {
  await setWindowSize(LOGIN_W, LOGIN_H)
}

export async function ensurePetWindowVisible() {
  const win = getPetTauriWindow()
  if (!win) return

  try {
    try {
      await win.unminimize()
    } catch {
      // optional
    }

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

    // 异常坐标（如 Windows 最小化时的 -32000）或窗口过小 → 强制居中并恢复尺寸
    const badPos = !isPlausibleWindowPosition(pos.x, pos.y)
    const tooSmall = size.width < 120 || size.height < 120
    if (badPos || tooSmall) {
      localStorage.removeItem('mochi_window_position')
      if (tooSmall) {
        await setWindowSize(LOGIN_W, LOGIN_H)
      }
      await win.center()
      await win.show()
      await win.setAlwaysOnTop(true)
      return
    }

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
      localStorage.removeItem('mochi_window_position')
      await win.center()
      return
    }

    const mon = (await currentMonitor()) ?? primary
    const minX = mon.position.x
    const minY = mon.position.y
    const maxX = mon.position.x + mon.size.width - size.width
    const maxY = mon.position.y + mon.size.height - size.height

    const x = Math.max(minX, Math.min(pos.x, maxX))
    const y = Math.max(minY, Math.min(pos.y, maxY))
    if (Math.abs(x - pos.x) > 2 || Math.abs(y - pos.y) > 2) {
      await win.setPosition(new PhysicalPosition(x, y))
    }
  } catch {
    try {
      await win.unminimize()
      await win.show()
      await win.center()
    } catch {
      // ignore
    }
  }
}

export async function initPetWindowChrome() {
  const win = getPetTauriWindow()
  if (!win) return
  try {
    await win.setShadow(false)
    try {
      await win.unminimize()
    } catch {
      // optional
    }
    await win.show()
    await win.setAlwaysOnTop(true)
    try {
      await win.setIgnoreCursorEvents(false)
    } catch {
      // optional
    }
    await win.setFocus()
  } catch {
    // optional
  }
}

export function syncBrowserSidePanelPlacement(): void {
  if (isTauri()) return
  const margin = 8
  const petX = window.screenX
  const expandedW = PET_WITH_SIDE_W
  const fitsRight = petX + expandedW <= window.screen.width - margin
  const fitsLeft = petX - PANEL_OFFSET >= margin
  if (fitsRight && !fitsLeft) {
    sidePanelOnLeft.value = false
    return
  }
  if (fitsLeft && !fitsRight) {
    sidePanelOnLeft.value = true
    return
  }
  sidePanelOnLeft.value = window.screenX > window.screen.width / 2
}

export const expandPetWindowForChat = openChatPanel
export const collapsePetWindowForChat = closeChatPanel
