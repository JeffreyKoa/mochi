import { playAudioBuffer, playBase64Audio, stopAllPlayback } from '@/services/voice'
import { opusStreamPlayer } from '@/services/opusStreamPlayer'

function mergeArrayBuffers(chunks: ArrayBuffer[]): ArrayBuffer {
  const total = chunks.reduce((n, c) => n + c.byteLength, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    out.set(new Uint8Array(chunk), offset)
    offset += chunk.byteLength
  }
  return out.buffer
}

/** Sequential MP3 (or other) chunk player for streaming TTS. */
export class TTSAudioQueue {
  private queue: Array<{ data: string; format: string }> = []
  private pendingBinary: ArrayBuffer[] = []
  private pendingFormat = 'mp3'
  private pumping = false
  private markedDone = false
  private onIdle: (() => void) | null = null
  private onFirstPlay: (() => void) | null = null
  private firstPlayFired = false
  private opusActive = false

  enqueue(data: string | ArrayBuffer, format = 'mp3', onFirstPlay?: () => void) {
    if (onFirstPlay) this.onFirstPlay = onFirstPlay
    if (format === 'opus' && data instanceof ArrayBuffer) {
      this.opusActive = true
      opusStreamPlayer.enqueue(data, onFirstPlay)
      return
    }
    if (data instanceof ArrayBuffer) {
      if (data.byteLength === 0) return
      this.pendingBinary.push(data)
      this.pendingFormat = format
      return
    }
    if (!data) return
    this.queue.push({ data, format })
    void this.pump()
  }

  markDone(onIdle?: () => void) {
    this.onIdle = onIdle ?? null
    this.markedDone = true
    void this.flushBinary()
    void this.awaitOpusAndFinish()
  }

  stop() {
    stopAllPlayback()
    opusStreamPlayer.stop()
    this.queue = []
    this.pendingBinary = []
    this.pumping = false
    this.markedDone = false
    this.opusActive = false
    this.onIdle = null
    this.onFirstPlay = null
    this.firstPlayFired = false
  }

  private finish() {
    const cb = this.onIdle
    this.onIdle = null
    cb?.()
  }

  private fireFirstPlay() {
    if (this.firstPlayFired) return
    this.firstPlayFired = true
    this.onFirstPlay?.()
    this.onFirstPlay = null
  }

  private async flushBinary() {
    if (this.pumping || this.pendingBinary.length === 0) {
      this.maybeFinish()
      return
    }
    this.pumping = true
    const merged = mergeArrayBuffers(this.pendingBinary)
    this.pendingBinary = []
    this.fireFirstPlay()
    try {
      await playAudioBuffer(merged, this.pendingFormat)
    } catch (e) {
      if (import.meta.env.DEV) console.warn('[tts] binary playback failed', e)
    }
    this.pumping = false
    if (this.pendingBinary.length > 0) {
      void this.flushBinary()
      return
    }
    if (this.queue.length > 0) {
      void this.pump()
      return
    }
    this.maybeFinish()
  }

  private async awaitOpusAndFinish() {
    if (!this.opusActive) {
      this.maybeFinish()
      return
    }
    await opusStreamPlayer.waitUntilIdle()
    this.opusActive = false
    this.maybeFinish()
  }

  private maybeFinish() {
    if (
      this.markedDone &&
      !this.pumping &&
      !this.opusActive &&
      this.queue.length === 0 &&
      this.pendingBinary.length === 0
    ) {
      this.finish()
    }
  }

  private async pump() {
    if (this.pumping) return
    this.pumping = true
    while (this.queue.length > 0) {
      const item = this.queue.shift()!
      this.fireFirstPlay()
      try {
        await playBase64Audio(item.data, item.format)
      } catch (e) {
        if (import.meta.env.DEV) console.warn('[tts] base64 playback failed', e)
      }
    }
    this.pumping = false
    if (this.pendingBinary.length > 0) {
      void this.flushBinary()
      return
    }
    this.maybeFinish()
  }
}
