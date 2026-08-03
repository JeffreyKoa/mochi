/** Event Loop 延迟探针：测量 setInterval 漂移，诊断主线程 Task Starvation。 */

const DEFAULT_INTERVAL_MS = 1000
const LAG_WARN_MS = 200

type LagListener = (lagMs: number) => void

let timer: ReturnType<typeof setInterval> | null = null
let expectedAt = 0
let currentLagMs = 0
let maxLagMs = 0
const listeners = new Set<LagListener>()

function tick() {
  const now = performance.now()
  const lag = Math.max(0, now - expectedAt)
  currentLagMs = Math.round(lag)
  if (currentLagMs > maxLagMs) maxLagMs = currentLagMs

  if (import.meta.env.DEV && currentLagMs >= LAG_WARN_MS) {
    console.warn('[eventLoop] lag_ms=%d (threshold=%d)', currentLagMs, LAG_WARN_MS)
  }

  for (const fn of listeners) {
    try {
      fn(currentLagMs)
    } catch (e) {
      console.warn('[eventLoop] listener error', e)
    }
  }

  expectedAt = now + probeIntervalMs
}

let probeIntervalMs = DEFAULT_INTERVAL_MS

/** 启动探针（DEV 或显式开启时调用）。 */
export function startEventLoopProbe(intervalMs = DEFAULT_INTERVAL_MS): void {
  if (timer) return
  probeIntervalMs = intervalMs
  expectedAt = performance.now() + intervalMs
  timer = setInterval(tick, intervalMs)
}

export function stopEventLoopProbe(): void {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  currentLagMs = 0
}

export function getEventLoopLagMs(): number {
  return currentLagMs
}

export function getEventLoopMaxLagMs(): number {
  return maxLagMs
}

export function resetEventLoopMaxLag(): void {
  maxLagMs = 0
}

export function onEventLoopLag(fn: LagListener): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export const EVENT_LOOP_LAG_WARN_MS = LAG_WARN_MS
