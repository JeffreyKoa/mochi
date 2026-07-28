import { playAudioBuffer, playBase64Audio, stopAllPlayback } from '@/services/voice'
import { opusStreamPlayer, isOpusDecodeSupported } from '@/services/opusStreamPlayer'
import { PCMPlayer } from '@/services/pcmPlayer'

export { isOpusDecodeSupported }

const TTS_PCM_SAMPLE_RATE = 22050

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
  /** Chunks for the current TTS sentence/segment (merged before playback). */
  private segmentBuffer: ArrayBuffer[] = []
  private segmentFormat = 'mp3'
  /** Completed segment blobs waiting for sequential playback. */
  private playQueue: ArrayBuffer[] = []
  private playFormat = 'mp3'
  private pumping = false
  private markedDone = false
  private onIdle: (() => void) | null = null
  private onFirstPlay: (() => void) | null = null
  private firstPlayFired = false
  private opusActive = false
  private pcmPlayer = new PCMPlayer(TTS_PCM_SAMPLE_RATE)
  private pcmActive = false
  private playbackStarted = false
  private lastSeq = 0
  private seqInitialized = false

  /** True if at least one audio chunk actually started playback this turn. */
  get hadPlayback(): boolean {
    return this.playbackStarted
  }

  resetTurn() {
    this.playbackStarted = false
    this.firstPlayFired = false
    this.lastSeq = 0
    this.seqInitialized = false
  }

  enqueue(data: string | ArrayBuffer, format = 'mp3', onFirstPlay?: () => void, seq?: number) {
    if (onFirstPlay) this.onFirstPlay = onFirstPlay
    if (typeof seq === 'number' && seq > 0) {
      this.trackSeq(seq)
    }
    if (format === 'opus' && data instanceof ArrayBuffer) {
      this.opusActive = true
      opusStreamPlayer.enqueue(data, onFirstPlay)
      return
    }
    if (format === 'pcm' && data instanceof ArrayBuffer) {
      if (data.byteLength === 0) return
      this.pcmActive = true
      this.fireFirstPlay()
      void this.pcmPlayer.enqueue(data)
      return
    }
    if (data instanceof ArrayBuffer) {
      if (data.byteLength === 0) return
      this.segmentBuffer.push(data)
      this.segmentFormat = format
      return
    }
    if (!data) return
    this.queue.push({ data, format })
    void this.pumpLegacy()
  }

  /** Flush current segment buffer into the sequential play queue. */
  flushSegment() {
    if (this.segmentBuffer.length === 0) return
    this.playQueue.push(mergeArrayBuffers(this.segmentBuffer))
    this.playFormat = this.segmentFormat
    this.segmentBuffer = []
    void this.drainPlayQueue()
  }

  markDone(onIdle?: () => void) {
    this.onIdle = onIdle ?? null
    this.markedDone = true
    this.flushSegment()
    if (this.pcmActive) {
      this.pcmPlayer.markDone(() => {
        this.pcmActive = false
        void this.drainPlayQueue()
        void this.awaitOpusAndFinish()
      })
      return
    }
    void this.drainPlayQueue()
    void this.awaitOpusAndFinish()
  }

  stop() {
    stopAllPlayback()
    opusStreamPlayer.stop()
    this.pcmPlayer.stop()
    this.queue = []
    this.segmentBuffer = []
    this.playQueue = []
    this.pumping = false
    this.markedDone = false
    this.opusActive = false
    this.pcmActive = false
    this.onIdle = null
    this.onFirstPlay = null
    this.firstPlayFired = false
  }

  private trackSeq(seq: number) {
    if (!this.seqInitialized) {
      this.seqInitialized = true
      this.lastSeq = seq
      return
    }
    if (seq > this.lastSeq + 1 && import.meta.env.DEV) {
      console.warn(`[tts] seq gap: expected ${this.lastSeq + 1}, got ${seq}`)
    }
    this.lastSeq = Math.max(this.lastSeq, seq)
  }

  private finish() {
    const cb = this.onIdle
    this.onIdle = null
    cb?.()
  }

  private fireFirstPlay() {
    if (this.firstPlayFired) return
    this.firstPlayFired = true
    this.playbackStarted = true
    this.onFirstPlay?.()
    this.onFirstPlay = null
  }

  private async drainPlayQueue() {
    if (this.pumping) return
    this.pumping = true
    while (this.playQueue.length > 0) {
      const merged = this.playQueue.shift()!
      this.fireFirstPlay()
      try {
        await playAudioBuffer(merged, this.playFormat)
      } catch (e) {
        if (import.meta.env.DEV) console.warn('[tts] segment playback failed', e)
      }
    }
    this.pumping = false
    if (this.playQueue.length > 0) {
      void this.drainPlayQueue()
      return
    }
    if (this.queue.length > 0) {
      void this.pumpLegacy()
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
    if (!this.playbackStarted && opusStreamPlayer.droppedFrameCount > 0) {
      if (import.meta.env.DEV) {
        console.warn('[tts] opus frames dropped, playback skipped')
      }
    }
    this.opusActive = false
    this.maybeFinish()
  }

  private maybeFinish() {
    if (
      this.markedDone &&
      !this.pumping &&
      !this.opusActive &&
      !this.pcmActive &&
      this.queue.length === 0 &&
      this.segmentBuffer.length === 0 &&
      this.playQueue.length === 0
    ) {
      this.finish()
    }
  }

  private async pumpLegacy() {
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
    if (this.playQueue.length > 0) {
      void this.drainPlayQueue()
      return
    }
    this.maybeFinish()
  }
}
