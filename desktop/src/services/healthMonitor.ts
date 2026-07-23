import { getApiBase } from '@/config'

const INTERVAL_MS = 3000
const MAX_ATTEMPTS = 30
const TIMEOUT_MS = 3000

function healthUrl(): string {
  const base = getApiBase()
  return base ? `${base}/health` : '/health'
}

/** 轻量探活：仅判断后端进程是否存活 */
export async function pingHealth(): Promise<boolean> {
  try {
    const res = await fetch(healthUrl(), { signal: AbortSignal.timeout(TIMEOUT_MS) })
    if (!res.ok) return false
    const data = (await res.json()) as { status?: string }
    return data.status === 'ok'
  } catch {
    return false
  }
}

type RecoveredHandler = () => void
type TickHandler = (attempt: number, up: boolean) => void

class HealthMonitor {
  private timer: ReturnType<typeof setInterval> | null = null
  private attempts = 0
  private onRecovered: RecoveredHandler | null = null
  private onTick: TickHandler | null = null

  get watching(): boolean {
    return this.timer !== null
  }

  start(onRecovered: RecoveredHandler, onTick?: TickHandler) {
    this.stop()
    this.onRecovered = onRecovered
    this.onTick = onTick ?? null
    this.attempts = 0
    void this.tick()
    this.timer = setInterval(() => void this.tick(), INTERVAL_MS)
  }

  stop() {
    if (this.timer) clearInterval(this.timer)
    this.timer = null
    this.attempts = 0
    this.onRecovered = null
    this.onTick = null
  }

  private async tick() {
    if (!this.timer) return
    if (this.attempts >= MAX_ATTEMPTS) {
      this.stop()
      return
    }
    this.attempts++
    const up = await pingHealth()
    this.onTick?.(this.attempts, up)
    if (up) {
      const recovered = this.onRecovered
      this.stop()
      recovered?.()
    }
  }
}

export const healthMonitor = new HealthMonitor()
