import { playBase64Audio, stopAllPlayback } from '@/services/voice'

/** Sequential MP3 (or other) chunk player for streaming TTS. */
export class TTSAudioQueue {
  private queue: Array<{ data: string; format: string }> = []
  private pumping = false
  private markedDone = false
  private onIdle: (() => void) | null = null

  enqueue(base64: string, format = 'mp3') {
    this.queue.push({ data: base64, format })
    void this.pump()
  }

  markDone(onIdle?: () => void) {
    this.onIdle = onIdle ?? null
    this.markedDone = true
    if (!this.pumping && this.queue.length === 0) {
      this.finish()
    }
  }

  stop() {
    stopAllPlayback()
    this.queue = []
    this.pumping = false
    this.markedDone = false
    this.onIdle = null
  }

  private finish() {
    const cb = this.onIdle
    this.onIdle = null
    cb?.()
  }

  private async pump() {
    if (this.pumping) return
    this.pumping = true
    while (this.queue.length > 0) {
      const item = this.queue.shift()!
      try {
        await playBase64Audio(item.data, item.format)
      } catch {
        // skip broken chunk, try next
      }
    }
    this.pumping = false
    if (this.markedDone && this.queue.length === 0) {
      this.finish()
    }
  }
}
