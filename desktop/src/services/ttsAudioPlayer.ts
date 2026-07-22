import { playBase64Audio, stopAllPlayback } from '@/services/voice'

/** Sequential MP3 (or other) chunk player for streaming TTS. */
export class TTSAudioQueue {
  private queue: Array<{ data: string; format: string }> = []
  private pumping = false
  private markedDone = false
  private onIdle: (() => void) | null = null
  private onFirstPlay: (() => void) | null = null
  private firstPlayFired = false

  enqueue(base64: string, format = 'mp3', onFirstPlay?: () => void) {
    if (onFirstPlay) this.onFirstPlay = onFirstPlay
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
    this.onFirstPlay = null
    this.firstPlayFired = false
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
      if (!this.firstPlayFired) {
        this.firstPlayFired = true
        this.onFirstPlay?.()
        this.onFirstPlay = null
      }
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
