/**
 * X-ASR 本地流式 STT：PCM → sidecar WebSocket → partial/final 文本。
 * 由 realtimeStore 在 VAD 边界驱动 begin/finish utterance。
 */
import { XAsrClient, type XAsrClientHandlers } from '@/services/xAsrClient'
import type { LocalSTTCallbacks } from '@/services/localStt'

/** beginUtterance 完成前暂存 PCM，避免首包丢失。 */
const MAX_PENDING_PCM = 80

export class XAsrSTT {
  private client: XAsrClient
  private callbacks: LocalSTTCallbacks | null = null
  private running = false
  private utteranceOpen = false
  private lastPartial = ''
  private pendingPcm: ArrayBuffer[] = []
  private beginFlight: Promise<void> | null = null
  /** 聚合 PCM，减少 WebSocket 小包开销。 */
  private aggBuffer = new Uint8Array(0)
  private readonly targetBytes: number

  constructor(
    private readonly wsUrl: string,
    private readonly sampleRate = 16000,
    chunkMs = 80,
  ) {
    this.client = new XAsrClient()
    // int16 mono：2 bytes/sample
    this.targetBytes = Math.max(320, Math.floor((this.sampleRate * 2 * chunkMs) / 1000))
  }

  get isRunning() {
    return this.running
  }

  get isUtteranceOpen() {
    return this.utteranceOpen
  }

  /** 与 sidecar 的长连接是否存活。 */
  get isSidecarConnected() {
    return this.client.connected
  }

  /** 在已有会话连接上 ping，避免探测时再开一条 WS。 */
  async pingSidecar(timeoutMs = 4000): Promise<boolean> {
    if (!this.client.connected) return false
    return this.client.ping(timeoutMs)
  }

  /** 连接 sidecar。 */
  async connect(): Promise<boolean> {
    const handlers: XAsrClientHandlers = {
      onPartial: (text) => {
        const trimmed = text.trim()
        if (!trimmed || trimmed === this.lastPartial) return
        this.lastPartial = trimmed
        this.callbacks?.onPartial(trimmed)
      },
      onError: (msg) => {
        this.callbacks?.onError?.(msg)
      },
    }
    this.client.setHandlers(handlers)
    return this.client.connect(this.wsUrl)
  }

  /** 注册回调；不自动开 utterance。 */
  prepare(callbacks: LocalSTTCallbacks) {
    this.callbacks = callbacks
    this.running = true
  }

  /** 确保 utterance 已开启（可并发调用，内部去重）。 */
  ensureUtterance(): Promise<void> {
    if (!this.running) return Promise.resolve()
    if (this.utteranceOpen) return Promise.resolve()
    if (this.beginFlight) return this.beginFlight
    this.beginFlight = this.beginUtterance().finally(() => {
      this.beginFlight = null
    })
    return this.beginFlight
  }

  /** VAD speech_start / 名字探针：开启一轮识别。 */
  async beginUtterance(): Promise<void> {
    if (!this.running) return
    if (this.utteranceOpen) {
      await this.client.resetSession()
    }
    this.lastPartial = ''
    this.pendingPcm = []
    this.aggBuffer = new Uint8Array(0)
    await this.client.startSession(this.sampleRate)
    this.utteranceOpen = true
    this.flushPendingPcm()
  }

  /** 持续喂 PCM（16kHz int16 mono）；session 未就绪时先缓冲。 */
  feedPcm(pcm: ArrayBuffer) {
    if (!this.running || pcm.byteLength === 0) return
    const chunk = new Uint8Array(pcm)
    const merged = new Uint8Array(this.aggBuffer.length + chunk.length)
    merged.set(this.aggBuffer, 0)
    merged.set(chunk, this.aggBuffer.length)

    let offset = 0
    while (offset + this.targetBytes <= merged.length) {
      const slice = merged.slice(offset, offset + this.targetBytes)
      this.dispatchPcm(slice.buffer.slice(slice.byteOffset, slice.byteOffset + slice.byteLength))
      offset += this.targetBytes
    }
    this.aggBuffer = merged.slice(offset)
  }

  /** 句末刷出剩余聚合 PCM。 */
  private flushAggRemainder() {
    if (this.aggBuffer.length === 0) return
    const tail = this.aggBuffer.slice()
    this.aggBuffer = new Uint8Array(0)
    this.dispatchPcm(tail.buffer.slice(tail.byteOffset, tail.byteOffset + tail.byteLength))
  }

  private dispatchPcm(pcm: ArrayBuffer) {
    if (pcm.byteLength === 0) return
    if (this.utteranceOpen) {
      this.client.sendPcm(pcm)
      return
    }
    if (this.pendingPcm.length < MAX_PENDING_PCM) {
      this.pendingPcm.push(pcm.slice(0))
    }
  }

  private flushPendingPcm() {
    if (!this.utteranceOpen) return
    for (const pcm of this.pendingPcm) {
      this.client.sendPcm(pcm)
    }
    this.pendingPcm = []
  }

  /** VAD speech_end / 提交：结束 utterance；triggerFinal 为 false 时仅返回文本。 */
  async finishUtterance(triggerFinal = true): Promise<string> {
    if (this.beginFlight) {
      try {
        await this.beginFlight
      } catch {
        // ignore
      }
    }
    if (!this.utteranceOpen) {
      return this.lastPartial
    }
    try {
      this.flushAggRemainder()
      const finalText = (await this.client.endSession()).trim()
      const text = finalText || this.lastPartial
      this.utteranceOpen = false
      this.pendingPcm = []
      this.aggBuffer = new Uint8Array(0)
      this.lastPartial = ''
      if (text && triggerFinal) {
        this.callbacks?.onFinal(text)
      }
      return text
    } catch (e) {
      this.utteranceOpen = false
      this.pendingPcm = []
      this.aggBuffer = new Uint8Array(0)
      const fallback = this.lastPartial
      this.lastPartial = ''
      if (fallback && triggerFinal) {
        this.callbacks?.onFinal(fallback)
      }
      this.callbacks?.onError?.(e instanceof Error ? e.message : 'x-asr finish failed')
      return fallback
    }
  }

  /** 名字探针取消：丢弃会话，不触发 onFinal。 */
  async cancelUtterance() {
    if (this.beginFlight) {
      try {
        await this.beginFlight
      } catch {
        // ignore
      }
    }
    if (!this.utteranceOpen) {
      this.pendingPcm = []
      this.aggBuffer = new Uint8Array(0)
      return
    }
    this.utteranceOpen = false
    this.pendingPcm = []
    this.aggBuffer = new Uint8Array(0)
    this.lastPartial = ''
    await this.client.resetSession()
  }

  stop() {
    this.running = false
    this.utteranceOpen = false
    this.pendingPcm = []
    this.aggBuffer = new Uint8Array(0)
    this.lastPartial = ''
    this.beginFlight = null
    this.callbacks = null
    this.client.close()
  }
}
