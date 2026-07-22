import { PhysicalPosition } from '@tauri-apps/api/dpi'
import type { Window } from '@tauri-apps/api/window'

const POSITION_KEY = 'mochi_window_position'

type TauriWindow = Window

export interface SavedPosition {
  x: number
  y: number
}

export interface RoamingBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

type RoamingHooks = {
  isPaused: () => boolean
  onWalkStart: (facing: 'left' | 'right') => void
  onWalkEnd: () => void
}

const ROAM_IDLE_MIN_MS = 3_000
const ROAM_IDLE_MAX_MS = 8_000
const ROAM_STEP_PX = 4
const ROAM_MIN_DISTANCE = 20
const ROAM_FRAME_MS = 16
const ROAM_FALLBACK_PX = 120

export function loadSavedPosition(): SavedPosition | null {
  try {
    const raw = localStorage.getItem(POSITION_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as SavedPosition
    if (typeof parsed.x === 'number' && typeof parsed.y === 'number') return parsed
  } catch {
    // ignore
  }
  return null
}

export async function restoreWindowPosition(win: TauriWindow): Promise<boolean> {
  const saved = loadSavedPosition()
  if (!saved) return false
  await win.setPosition(new PhysicalPosition(saved.x, saved.y))
  return true
}

export async function saveWindowPosition(win: TauriWindow): Promise<void> {
  const pos = await win.outerPosition()
  localStorage.setItem(POSITION_KEY, JSON.stringify({ x: pos.x, y: pos.y }))
}

async function getRoamingBounds(win: TauriWindow): Promise<RoamingBounds | null> {
  try {
    const { currentMonitor, PrimaryMonitor, availableMonitors } = await import(
      '@tauri-apps/api/window'
    )
    let monitor = await currentMonitor()
    if (!monitor) {
      const all = await availableMonitors()
      monitor = all[0] ?? (await PrimaryMonitor())
    }
    if (!monitor) return null

    const size = await win.outerSize()
    const pos = monitor.position
    const mon = monitor.size
    const margin = 8
    const maxX = pos.x + mon.width - size.width - margin
    const maxY = pos.y + mon.height - size.height - margin
    if (maxX <= pos.x + margin || maxY <= pos.y + margin) return null

    return {
      minX: pos.x + margin,
      minY: pos.y + margin,
      maxX,
      maxY,
    }
  } catch (e) {
    console.warn('[roam] bounds', e)
    return null
  }
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}

export class PetRoamer {
  private win: TauriWindow | null = null
  private hooks: RoamingHooks | null = null
  private idleTimer: ReturnType<typeof setTimeout> | null = null
  private moving = false
  private paused = false
  private stopped = false

  start(win: TauriWindow, hooks: RoamingHooks) {
    this.win = win
    this.hooks = hooks
    this.stopped = false
    this.paused = false
    console.info('[roam] started')
    this.scheduleIdle(1500)
  }

  stop() {
    this.stopped = true
    this.paused = true
    this.clearIdle()
    this.moving = false
  }

  pause() {
    this.paused = true
    this.clearIdle()
  }

  resume() {
    if (this.stopped) return
    this.paused = false
    if (!this.moving) this.scheduleIdle(800)
  }

  get isMoving() {
    return this.moving
  }

  private clearIdle() {
    if (this.idleTimer) {
      clearTimeout(this.idleTimer)
      this.idleTimer = null
    }
  }

  private scheduleIdle(firstDelayMs?: number) {
    if (this.stopped || this.paused || this.moving) return
    this.clearIdle()
    const delay =
      firstDelayMs ??
      ROAM_IDLE_MIN_MS + Math.random() * (ROAM_IDLE_MAX_MS - ROAM_IDLE_MIN_MS)
    this.idleTimer = setTimeout(() => {
      void this.roamOnce()
    }, delay)
  }

  private async simpleStroll() {
    if (!this.win || !this.hooks) return
    const pos = await this.win.outerPosition()
    const goRight = Math.random() > 0.5
    const delta = goRight ? ROAM_FALLBACK_PX : -ROAM_FALLBACK_PX
    const facing: 'left' | 'right' = goRight ? 'right' : 'left'

    this.moving = true
    this.hooks.onWalkStart(facing)

    const steps = Math.ceil(Math.abs(delta) / ROAM_STEP_PX)
    for (let i = 1; i <= steps; i++) {
      if (this.stopped || this.paused || this.hooks.isPaused()) break
      const x = Math.round(pos.x + (delta * i) / steps)
      const y = pos.y
      try {
        await this.win.setPosition(new PhysicalPosition(x, y))
      } catch (e) {
        console.warn('[roam] move', e)
        break
      }
      await sleep(ROAM_FRAME_MS)
    }

    if (!this.stopped && !this.paused) {
      try {
        await saveWindowPosition(this.win)
      } catch {
        // ignore
      }
    }

    this.moving = false
    this.hooks.onWalkEnd()
    this.scheduleIdle()
  }

  private async roamOnce() {
    if (!this.win || !this.hooks || this.stopped || this.paused || this.hooks.isPaused()) {
      this.scheduleIdle()
      return
    }

    const bounds = await getRoamingBounds(this.win)
    if (!bounds) {
      await this.simpleStroll()
      return
    }

    const pos = await this.win.outerPosition()
    let targetX = bounds.minX + Math.random() * (bounds.maxX - bounds.minX)
    let targetY = bounds.minY + Math.random() * (bounds.maxY - bounds.minY)
    targetY = clamp(targetY, bounds.minY, bounds.maxY)
    targetX = clamp(targetX, bounds.minX, bounds.maxX)

    const dx = targetX - pos.x
    const dy = targetY - pos.y
    if (Math.hypot(dx, dy) < ROAM_MIN_DISTANCE) {
      await this.simpleStroll()
      return
    }

    const facing: 'left' | 'right' = dx >= 0 ? 'right' : 'left'
    this.moving = true
    this.hooks.onWalkStart(facing)

    let x = pos.x
    let y = pos.y
    const total = Math.hypot(dx, dy)
    const steps = Math.ceil(total / ROAM_STEP_PX)
    const stepX = dx / steps
    const stepY = dy / steps

    for (let i = 0; i < steps; i++) {
      if (this.stopped || this.paused || this.hooks.isPaused()) break
      x += stepX
      y += stepY
      x = clamp(x, bounds.minX, bounds.maxX)
      y = clamp(y, bounds.minY, bounds.maxY)
      try {
        await this.win.setPosition(new PhysicalPosition(Math.round(x), Math.round(y)))
      } catch (e) {
        console.warn('[roam] move', e)
        break
      }
      await sleep(ROAM_FRAME_MS)
    }

    if (!this.stopped && !this.paused) {
      try {
        await saveWindowPosition(this.win)
      } catch {
        // ignore
      }
    }

    this.moving = false
    this.hooks.onWalkEnd()
    this.scheduleIdle()
  }
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function canRoam(_energy: number, isChatOpen: boolean, isDragging: boolean): boolean {
  if (isChatOpen || isDragging) return false
  return true
}
