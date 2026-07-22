/**
 * Silero VAD (via @ricky0123/vad-web) + energy fallback for turn-taking.
 */
import {
  FrameProcessor,
  Message,
  getDefaultRealTimeVADOptions,
  type FrameProcessorOptions,
} from '@ricky0123/vad-web'
import { SileroV5 } from '@ricky0123/vad-web/dist/models/v5'
import * as ort from 'onnxruntime-web/wasm'

const VAD_VER = '0.0.30'
const VAD_BASE = `https://cdn.jsdelivr.net/npm/@ricky0123/vad-web@${VAD_VER}/dist/`
const ORT_BASE = 'https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/'

const FRAME_SAMPLES = 512
const MS_PER_FRAME = FRAME_SAMPLES / 16

export type VADEvent = 'speech_start' | 'speech_end'

/** Runs energy VAD always; adds Silero when model loads. */
export class HybridSpeechVad {
  private energy: EnergySpeechVad
  private silero: SileroSpeechVad | null = null
  private playbackMode = false

  constructor(onEvent: (ev: VADEvent) => void) {
    this.energy = new EnergySpeechVad(onEvent, 800, 250, 0.025)
    this.silero = new SileroSpeechVad(onEvent, {
      positiveSpeechThreshold: 0.35,
      negativeSpeechThreshold: 0.25,
      redemptionMs: 500,
      minSpeechMs: 250,
      preSpeechPadMs: 200,
    })
  }

  async init(): Promise<void> {
    const ok = await this.silero!.init()
    if (!ok) this.silero = null
  }

  /** During TTS playback: disable Silero and raise energy threshold to reduce echo false triggers. */
  setPlaybackMode(playing: boolean) {
    this.playbackMode = playing
    this.energy.setPeakThreshold(playing ? 0.08 : 0.025)
    if (playing) {
      this.silero?.reset()
    }
  }

  feed(samples: Float32Array) {
    if (!this.playbackMode) {
      this.silero?.feed(samples)
    }
    this.energy.feed(samples)
  }

  reset() {
    this.energy.reset()
    this.silero?.reset()
  }

  destroy() {
    this.energy.destroy()
    this.silero?.destroy()
    this.silero = null
  }
}

export class SileroSpeechVad {
  private processor: FrameProcessor | null = null
  private pending = new Float32Array(0)
  private ready = false

  constructor(
    private onEvent: (ev: VADEvent) => void,
    private options: Partial<FrameProcessorOptions> = {},
  ) {}

  async init(): Promise<boolean> {
    try {
      ort.env.logLevel = 'error'
      ort.env.wasm.wasmPaths = ORT_BASE

      const model = await SileroV5.new(ort, async () => {
        const res = await fetch(`${VAD_BASE}silero_vad_v5.onnx`)
        if (!res.ok) throw new Error(`model fetch ${res.status}`)
        return res.arrayBuffer()
      })

      this.processor = new FrameProcessor(
        model.process.bind(model),
        model.reset_state.bind(model),
        {
          ...getDefaultRealTimeVADOptions('v5'),
          ...this.options,
        },
        MS_PER_FRAME,
      )
      this.processor.resume()
      this.ready = true
      return true
    } catch (e) {
      console.warn('[SileroVAD] init failed, using energy VAD only', e)
      return false
    }
  }

  feed(samples: Float32Array) {
    if (!this.ready || !this.processor) return

    const merged = new Float32Array(this.pending.length + samples.length)
    merged.set(this.pending)
    merged.set(samples, this.pending.length)
    this.pending = merged

    while (this.pending.length >= FRAME_SAMPLES) {
      const frame = this.pending.slice(0, FRAME_SAMPLES)
      this.pending = this.pending.slice(FRAME_SAMPLES)
      void this.processor.process(frame, (ev) => {
        switch (ev.msg) {
          case Message.SpeechStart:
          case Message.SpeechRealStart:
            this.onEvent('speech_start')
            break
          case Message.SpeechEnd:
            this.onEvent('speech_end')
            break
        }
      })
    }
  }

  reset() {
    this.pending = new Float32Array(0)
    this.processor?.reset()
    this.processor?.resume()
  }

  destroy() {
    this.processor = null
    this.ready = false
    this.pending = new Float32Array(0)
  }
}

export class EnergySpeechVad {
  private speaking = false
  private speechMs = 0
  private silenceMs = 0
  private readonly frameMs = 20

  constructor(
    private onEvent: (ev: VADEvent) => void,
    private silenceThresholdMs = 800,
    private minSpeechMs = 250,
    private peakThreshold = 0.008,
  ) {}

  setPeakThreshold(threshold: number) {
    this.peakThreshold = threshold
  }

  feed(samples: Float32Array) {
    let peak = 0
    for (let i = 0; i < samples.length; i++) {
      peak = Math.max(peak, Math.abs(samples[i]))
    }

    if (peak >= this.peakThreshold) {
      if (!this.speaking) {
        this.speaking = true
        this.speechMs = 0
        this.onEvent('speech_start')
      }
      this.silenceMs = 0
      this.speechMs += this.frameMs
    } else if (this.speaking) {
      this.silenceMs += this.frameMs
      if (this.silenceMs >= this.silenceThresholdMs && this.speechMs >= this.minSpeechMs) {
        this.speaking = false
        this.speechMs = 0
        this.silenceMs = 0
        this.onEvent('speech_end')
      }
    }
  }

  reset() {
    this.speaking = false
    this.speechMs = 0
    this.silenceMs = 0
  }

  destroy() {}
}

export function pcmToFloat(pcm: ArrayBuffer): Float32Array {
  const view = new DataView(pcm)
  const n = view.byteLength / 2
  const out = new Float32Array(n)
  for (let i = 0; i < n; i++) {
    out[i] = view.getInt16(i * 2, true) / 32768
  }
  return out
}
