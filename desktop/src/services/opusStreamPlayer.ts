/**
 * Stream player for Opus audio packets using WebCodecs AudioDecoder + Web Audio API.
 * Supports sub-10ms decoding latency, smooth queueing, and instant barge-in stop.
 */

import { emitLipSync } from './voice'

export function isOpusDecodeSupported(): boolean {
  if (typeof AudioDecoder === 'undefined') return false
  try {
    const probe = new AudioDecoder({
      output: () => {},
      error: () => {},
    })
    probe.configure({ codec: 'opus', sampleRate: 48000, numberOfChannels: 1 })
    probe.close()
    return true
  } catch {
    return false
  }
}

export class OpusStreamPlayer {
  private audioCtx: AudioContext | null = null
  private decoder: AudioDecoder | null = null
  private nextStartTime = 0
  private timestampUs = 0
  private isPlaying = false
  private activeSources: AudioBufferSourceNode[] = []
  private onFirstPlay: (() => void) | null = null
  private firstPlayFired = false
  private sampleRate = 48000
  private gainNode: GainNode | null = null
  private analyser: AnalyserNode | null = null
  private lipSyncRaf: number | null = null

  constructor() {
    this.initDecoder()
  }

  private droppedFrames = 0

  get droppedFrameCount(): number {
    return this.droppedFrames
  }

  private initDecoder() {
    if (typeof AudioDecoder === 'undefined') {
      if (import.meta.env.DEV) {
        console.warn('[OpusStreamPlayer] WebCodecs AudioDecoder is not supported in this environment.')
      }
      return
    }

    try {
      this.decoder = new AudioDecoder({
        output: (frame) => this.onAudioFrame(frame),
        error: (e) => {
          if (import.meta.env.DEV) {
            console.error('[OpusStreamPlayer] AudioDecoder error:', e)
          }
        },
      })
      this.decoder.configure({
        codec: 'opus',
        sampleRate: 48000,
        numberOfChannels: 1,
      })
    } catch (e) {
      if (import.meta.env.DEV) {
        console.warn('[OpusStreamPlayer] Failed to configure AudioDecoder:', e)
      }
      this.decoder = null
    }
  }

  private getAudioContext(): AudioContext {
    if (!this.audioCtx) {
      const AudioCtx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      this.audioCtx = new AudioCtx({ sampleRate: 48000 })
    }
    if (this.audioCtx.state === 'suspended') {
      void this.audioCtx.resume()
    }
    return this.audioCtx
  }

  private ensureOutputChain(ctx: AudioContext) {
    if (this.gainNode && this.analyser) return
    this.gainNode = ctx.createGain()
    this.analyser = ctx.createAnalyser()
    this.analyser.fftSize = 256
    this.gainNode.connect(this.analyser)
    this.analyser.connect(ctx.destination)
  }

  private hasPendingAudio(ctx: AudioContext): boolean {
    return this.activeSources.length > 0 || this.nextStartTime > ctx.currentTime + 0.02
  }

  private startLipSyncLoop() {
    if (this.lipSyncRaf != null || !this.analyser) return
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount)
    const tick = () => {
      const ctx = this.audioCtx
      if (!this.analyser || !ctx || !this.hasPendingAudio(ctx)) {
        this.stopLipSyncLoop()
        return
      }
      this.analyser.getByteFrequencyData(dataArray)
      const avg = dataArray.reduce((a, b) => a + b, 0) / dataArray.length / 255
      emitLipSync(Math.min(1, avg * 2.5))
      this.lipSyncRaf = requestAnimationFrame(tick)
    }
    this.lipSyncRaf = requestAnimationFrame(tick)
  }

  private stopLipSyncLoop() {
    if (this.lipSyncRaf != null) {
      cancelAnimationFrame(this.lipSyncRaf)
      this.lipSyncRaf = null
    }
    emitLipSync(0)
  }

  enqueue(data: ArrayBuffer, onFirstPlay?: () => void) {
    if (onFirstPlay && !this.firstPlayFired) {
      this.onFirstPlay = onFirstPlay
    }

    if (!this.decoder || this.decoder.state === 'closed') {
      this.initDecoder()
    }

    if (!this.decoder || this.decoder.state !== 'configured') {
      this.droppedFrames++
      if (import.meta.env.DEV) {
        console.warn('[OpusStreamPlayer] AudioDecoder not ready, dropping frame')
      }
      return
    }

    try {
      const chunk = new EncodedAudioChunk({
        type: 'key',
        timestamp: this.timestampUs,
        duration: 20000, // 20ms frame
        data,
      })
      this.timestampUs += 20000
      this.decoder.decode(chunk)
    } catch (e) {
      if (import.meta.env.DEV) {
        console.warn('[OpusStreamPlayer] Decode error:', e)
      }
    }
  }

  private onAudioFrame(frame: AudioData) {
    try {
      if (!this.firstPlayFired) {
        this.firstPlayFired = true
        this.onFirstPlay?.()
        this.onFirstPlay = null
      }

      const ctx = this.getAudioContext()
      this.ensureOutputChain(ctx)
      const numberOfChannels = frame.numberOfChannels
      const numberOfFrames = frame.numberOfFrames
      const sampleRate = frame.sampleRate || this.sampleRate

      const buffer = ctx.createBuffer(numberOfChannels, numberOfFrames, sampleRate)
      for (let ch = 0; ch < numberOfChannels; ch++) {
        const channelData = new Float32Array(numberOfFrames)
        frame.copyTo(channelData, { planeIndex: ch })
        buffer.copyToChannel(channelData, ch)
      }

      const source = ctx.createBufferSource()
      source.buffer = buffer
      source.connect(this.gainNode!)

      const currentTime = ctx.currentTime
      if (this.nextStartTime < currentTime) {
        this.nextStartTime = currentTime + 0.01 // 10ms Jitter Buffer offset
      }

      source.start(this.nextStartTime)
      this.nextStartTime += buffer.duration
      this.isPlaying = true
      this.startLipSyncLoop()

      this.activeSources.push(source)
      source.onended = () => {
        const idx = this.activeSources.indexOf(source)
        if (idx >= 0) this.activeSources.splice(idx, 1)
        if (this.activeSources.length === 0) {
          this.isPlaying = false
          const audioCtx = this.audioCtx
          if (!audioCtx || !this.hasPendingAudio(audioCtx)) {
            this.stopLipSyncLoop()
          }
        }
      }
    } finally {
      frame.close()
    }
  }

  stop() {
    this.stopLipSyncLoop()
    this.droppedFrames = 0
    this.activeSources.forEach((src) => {
      try {
        src.stop()
        src.disconnect()
      } catch {
        // ignore
      }
    })
    this.activeSources = []

    if (this.decoder && this.decoder.state === 'configured') {
      try {
        this.decoder.reset()
        this.decoder.configure({
          codec: 'opus',
          sampleRate: 48000,
          numberOfChannels: 1,
        })
      } catch {
        // ignore
      }
    }

    this.nextStartTime = 0
    this.timestampUs = 0
    this.isPlaying = false
    this.firstPlayFired = false
    this.onFirstPlay = null
  }

  getIsPlaying(): boolean {
    return this.isPlaying
  }

  /** Resolves when all scheduled audio sources have finished playing. */
  waitUntilIdle(timeoutMs = 60000): Promise<void> {
    if (this.activeSources.length === 0 && !this.isPlaying) {
      this.stopLipSyncLoop()
      return Promise.resolve()
    }
    return new Promise((resolve) => {
      const deadline = Date.now() + timeoutMs
      const poll = () => {
        const ctx = this.audioCtx
        if (this.activeSources.length === 0 && !this.isPlaying && (!ctx || !this.hasPendingAudio(ctx))) {
          this.stopLipSyncLoop()
          resolve()
          return
        }
        if (Date.now() >= deadline) {
          if (import.meta.env.DEV) {
            console.warn('[OpusStreamPlayer] waitUntilIdle timed out')
          }
          resolve()
          return
        }
        setTimeout(poll, 30)
      }
      poll()
    })
  }
}

export const opusStreamPlayer = new OpusStreamPlayer()
