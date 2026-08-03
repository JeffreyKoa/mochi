import type { TurnMetrics } from '@/services/realtimeSession'

/** 开发态延迟基准：收集 turn_metrics 并输出 P50/P95。 */
const MAX_SAMPLES = 50

interface Sample {
  at: number
  playbackStartMs: number
  audioEndMs: number
  llmFirstTokenMs: number
}

const samples: Sample[] = []

function percentile(values: number[], p: number): number {
  if (values.length === 0) return -1
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)
  return sorted[idx]
}

/** 记录一轮 metrics（仅 DEV）。 */
export function recordTurnMetricsBaseline(metrics: TurnMetrics): void {
  if (!import.meta.env.DEV) return

  const playback = metrics.playbackStartMs >= 0 ? metrics.playbackStartMs : metrics.ttsFirstByteMs
  samples.push({
    at: Date.now(),
    playbackStartMs: playback,
    audioEndMs: metrics.audioEndMs,
    llmFirstTokenMs: metrics.llmFirstTokenMs,
  })
  if (samples.length > MAX_SAMPLES) samples.shift()

  if (samples.length >= 10 && samples.length % 10 === 0) {
    logBaselineSummary()
  }
}

export function logBaselineSummary(): void {
  if (!import.meta.env.DEV || samples.length === 0) return

  const playback = samples.map((s) => s.playbackStartMs).filter((v) => v >= 0)
  const llm = samples.map((s) => s.llmFirstTokenMs).filter((v) => v >= 0)

  console.info(
    '[baseline] n=%d playback P50=%d P95=%d | llmFirstToken P50=%d P95=%d',
    samples.length,
    percentile(playback, 50),
    percentile(playback, 95),
    percentile(llm, 50),
    percentile(llm, 95),
  )
}

export function getBaselineSampleCount(): number {
  return samples.length
}
