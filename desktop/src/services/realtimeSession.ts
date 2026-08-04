import { getApiBase, getToken } from './api'
import { isOpusDecodeSupported } from './ttsAudioPlayer'
import { probeAecEnabled } from './pcmCapture'

export type RealtimeEvent =
  | { type: 'session_start'; sessionId: string }
  | { type: 'vad'; event: 'speech_start' | 'speech_end' }
  | { type: 'asr_partial'; text: string; sentenceEnd?: boolean }
  | { type: 'asr_final'; text: string }
  | { type: 'llm_token'; token: string }
  | { type: 'llm_done'; text: string }
  | { type: 'tts_audio'; pcm: string; audioBuffer?: ArrayBuffer; format: string; seq: number }
  | { type: 'tts_segment_done' }
  | { type: 'tts_stream_start'; codec: string; sampleRate: number; channels: number; frameMs: number; bitrate: number }
  | { type: 'tts_done' }
  | { type: 'interrupted' }
  | { type: 'turn_ack' }
  | { type: 'turn_metrics'; metrics: TurnMetrics }
  | { type: 'animation'; state: string }
  | { type: 'barge_in_config'; echoGuardMs: number; peakThreshold: number; bargeInMs: number; aecEnabled: boolean }
  | { type: 'vision_pause_hint'; expression: string; composing: boolean; tier: string }
  | { type: 'proactive_message'; message: string; animation?: string; reminderId?: number; source?: string }
  | { type: 'error'; code: string; message: string }
  | { type: 'connected' }
  | { type: 'disconnected' }

type Listener = (ev: RealtimeEvent) => void

export interface TurnMetrics {
  audioEndMs: number
  asrFinalMs: number
  visionMs: number
  perceiveParallelMs: number
  llmFirstTokenMs: number
  llmFirstSentenceMs: number
  ttsFirstByteMs: number
  playbackStartMs: number
  fillerPlayedMs: number
}

export class RealtimeSession {
  private ws: WebSocket | null = null
  private listeners = new Set<Listener>()
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  /** Suppress disconnected event when replacing an existing socket during connect(). */
  private replacing = false

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      const token = getToken()
      if (!token) {
        reject(new Error('not logged in'))
        return
      }

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const base = getApiBase()
      const url = base
        ? `${base.replace(/^http/, 'ws')}/ws/voice?token=${encodeURIComponent(token)}`
        : `${proto}//${window.location.host}/ws/voice?token=${encodeURIComponent(token)}`

      if (this.ws) {
        this.replacing = true
        this.ws.onopen = null
        this.ws.onmessage = null
        this.ws.onerror = null
        this.ws.onclose = null
        this.ws.close()
        this.ws = null
      }

      this.ws = new WebSocket(url)
      this.ws.binaryType = 'arraybuffer'

      this.ws.onopen = () => {
        this.replacing = false
        this.startHeartbeat()
        this.emit({ type: 'connected' })
        resolve()
      }

      this.ws.onmessage = (e) => {
        if (e.data instanceof ArrayBuffer) {
          this.handleBinaryMessage(e.data)
          return
        }
        if (e.data instanceof Blob) {
          void e.data.arrayBuffer().then((buf) => this.handleBinaryMessage(buf))
          return
        }
        try {
          const msg = JSON.parse(e.data as string)
          this.dispatch(msg.type, msg.data)
        } catch {
          // ignore
        }
      }

      this.ws.onclose = () => {
        this.stopHeartbeat()
        if (this.replacing) {
          this.replacing = false
          return
        }
        this.emit({ type: 'disconnected' })
      }

      this.ws.onerror = () => {
        if (!this.replacing) {
          reject(new Error('websocket error'))
        }
      }
    })
  }

  disconnect() {
    this.stopHeartbeat()
    this.ws?.close()
    this.ws = null
  }

  sendAudio(pcmBase64: string, seq: number) {
    this.send('audio', { pcm: pcmBase64, seq })
  }

  sendAudioStart(): boolean {
    return this.send('audio_start', {})
  }

  sendAudioEnd(): boolean {
    return this.send('audio_end', {})
  }

  /** 上传语音 turn JPEG（base64，不含 data: 前缀）。 */
  sendVisionFrame(
    jpegBase64: string,
    options?: {
      seq?: number
      reason?: 'speech_start' | 'audio_end' | 'object_refresh' | 'pause_probe' | 'glance'
      partialText?: string
      faceProbe?: { match: boolean; score: number; detected: boolean }
    },
  ): boolean {
    const payload: {
      jpeg: string
      seq: number
      reason?: string
      partial_text?: string
      face_probe?: { match: boolean; score: number; detected: boolean }
    } = {
      jpeg: jpegBase64,
      seq: options?.seq ?? Date.now(),
    }
    if (options?.reason) payload.reason = options.reason
    if (options?.partialText) payload.partial_text = options.partialText
    if (options?.faceProbe) payload.face_probe = options.faceProbe
    return this.send('vision_frame', payload)
  }

  sendInterrupt(): boolean {
    return this.send('interrupt', {})
  }

  sendTextInput(text: string, options?: { voiceReply?: boolean }): boolean {
    const data: { text: string; voice_reply?: boolean } = { text }
    if (options?.voiceReply) {
      data.voice_reply = true
    }
    return this.send('text_input', data)
  }

  /** 仅 TTS 播报已有文案（在场闲聊等，不走 LLM）。 */
  sendSpeakOnly(text: string): boolean {
    return this.send('speak_only', { text })
  }

  async sendClientCaps(options?: { localTts?: boolean }): Promise<boolean> {
    const aecEnabled = await probeAecEnabled()
    return this.send('client_caps', {
      opus_decode: isOpusDecodeSupported(),
      aec_enabled: aecEnabled,
      local_tts: options?.localTts ?? false,
    })
  }

  sendPrewarm(): boolean {
    return this.send('prewarm', {})
  }

  sendPlaybackMark(atMs: number): boolean {
    return this.send('playback_mark', { at_ms: atMs })
  }

  /** 场景①：非主人直接对 Mochi 说话，请求服务端 TTS 拒答。 */
  sendNonOwnerTurn(): boolean {
    return this.send('non_owner_turn', {})
  }

  /** 取消已 audio_start 但未上传有效音频的 utterance。 */
  sendUtteranceCancel(): boolean {
    return this.send('utterance_cancel', {})
  }

  sendTurnAck(): boolean {
    return this.send('turn_ack', {})
  }

  isOpen(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  on(listener: Listener) {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  private send(type: string, data: unknown): boolean {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, data, ts: Date.now() }))
      return true
    }
    return false
  }

  private dispatch(type: string, data: Record<string, unknown>) {
    switch (type) {
      case 'session_start':
        this.emit({ type: 'session_start', sessionId: String(data.session_id) })
        break
      case 'vad':
        this.emit({ type: 'vad', event: data.event as 'speech_start' | 'speech_end' })
        break
      case 'asr_partial':
        this.emit({
          type: 'asr_partial',
          text: String(data.text),
          sentenceEnd: Boolean(data.sentence_end),
        })
        break
      case 'asr_final':
        this.emit({ type: 'asr_final', text: String(data.text) })
        break
      case 'llm_token':
        this.emit({ type: 'llm_token', token: String(data.token) })
        break
      case 'llm_done':
        this.emit({ type: 'llm_done', text: String(data.text) })
        break
      case 'tts_audio':
        this.emit({
          type: 'tts_audio',
          pcm: String(data.pcm),
          format: String(data.format || 'mp3'),
          seq: Number(data.seq),
        })
        break
      case 'tts_segment_done':
        this.emit({ type: 'tts_segment_done' })
        break
      case 'tts_stream_start':
        this.emit({
          type: 'tts_stream_start',
          codec: String(data.codec || 'opus'),
          sampleRate: Number(data.sample_rate || 48000),
          channels: Number(data.channels || 1),
          frameMs: Number(data.frame_ms || 20),
          bitrate: Number(data.bitrate || 24000),
        })
        break
      case 'tts_done':
        this.emit({ type: 'tts_done' })
        break
      case 'interrupted':
        this.emit({ type: 'interrupted' })
        break
      case 'turn_ack':
        this.emit({ type: 'turn_ack' })
        break
      case 'turn_metrics':
        this.emit({
          type: 'turn_metrics',
          metrics: {
            audioEndMs: Number(data.audio_end_ms ?? -1),
            asrFinalMs: Number(data.asr_final_ms ?? -1),
            visionMs: Number(data.vision_ms ?? -1),
            perceiveParallelMs: Number(data.perceive_parallel_ms ?? -1),
            llmFirstTokenMs: Number(data.llm_first_token_ms ?? -1),
            llmFirstSentenceMs: Number(data.llm_first_sentence_ms ?? -1),
            ttsFirstByteMs: Number(data.tts_first_byte_ms ?? -1),
            playbackStartMs: Number(data.playback_start_ms ?? -1),
            fillerPlayedMs: Number(data.filler_played_ms ?? -1),
          },
        })
        break
      case 'animation':
        this.emit({ type: 'animation', state: String(data.state) })
        break
      case 'barge_in_config':
        this.emit({
          type: 'barge_in_config',
          echoGuardMs: Number(data.echo_guard_ms ?? 1800),
          peakThreshold: Number(data.peak_threshold ?? 0.06),
          bargeInMs: Number(data.barge_in_ms ?? 800),
          aecEnabled: Boolean(data.aec_enabled),
        })
        break
      case 'vision_pause_hint':
        this.emit({
          type: 'vision_pause_hint',
          expression: String(data.expression ?? ''),
          composing: Boolean(data.composing),
          tier: String(data.tier ?? 'tier0'),
        })
        break
      case 'proactive_message':
        this.emit({
          type: 'proactive_message',
          message: String(data.message ?? ''),
          animation: typeof data.animation === 'string' ? data.animation : undefined,
          reminderId: typeof data.reminder_id === 'number' ? data.reminder_id : undefined,
          source: typeof data.source === 'string' ? data.source : undefined,
        })
        break
      case 'error':
        this.emit({ type: 'error', code: String(data.code), message: String(data.message) })
        break
    }
  }

  private handleBinaryMessage(buffer: ArrayBuffer) {
    if (buffer.byteLength < 10) return
    const view = new DataView(buffer)
    const msgType = view.getUint8(0)
    if (msgType === 0x01) {
      const formatByte = view.getUint8(1)
      const format = formatByte === 0x03 ? 'opus' : formatByte === 0x02 ? 'pcm' : 'mp3'
      const high = view.getUint32(2)
      const low = view.getUint32(6)
      const seq = high * 4294967296 + low
      const audioBuffer = buffer.slice(10)

      this.emit({
        type: 'tts_audio',
        pcm: '',
        audioBuffer,
        format,
        seq,
      })
    }
  }

  private emit(ev: RealtimeEvent) {
    this.listeners.forEach((l) => l(ev))
  }

  private startHeartbeat() {
    this.heartbeatTimer = setInterval(() => {
      this.send('heartbeat', { ts: Date.now() })
    }, 30000)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) clearInterval(this.heartbeatTimer)
  }
}

export const realtimeSession = new RealtimeSession()
