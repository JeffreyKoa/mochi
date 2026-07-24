import { getApiBase, getToken } from './api'

export type RealtimeEvent =
  | { type: 'session_start'; sessionId: string }
  | { type: 'vad'; event: 'speech_start' | 'speech_end' }
  | { type: 'asr_partial'; text: string; sentenceEnd?: boolean }
  | { type: 'asr_final'; text: string }
  | { type: 'llm_token'; token: string }
  | { type: 'llm_done'; text: string }
  | { type: 'tts_audio'; pcm: string; format: string; seq: number }
  | { type: 'tts_done' }
  | { type: 'interrupted' }
  | { type: 'turn_ack' }
  | { type: 'turn_metrics'; metrics: TurnMetrics }
  | { type: 'animation'; state: string }
  | { type: 'proactive_message'; message: string; animation?: string; reminderId?: number }
  | { type: 'error'; code: string; message: string }
  | { type: 'connected' }
  | { type: 'disconnected' }

type Listener = (ev: RealtimeEvent) => void

export interface TurnMetrics {
  audioEndMs: number
  asrFinalMs: number
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
        this.ws.onopen = null
        this.ws.onmessage = null
        this.ws.onerror = null
        this.ws.onclose = null
        this.ws.close()
        this.ws = null
      }

      this.ws = new WebSocket(url)

      this.ws.onopen = () => {
        this.startHeartbeat()
        this.emit({ type: 'connected' })
        resolve()
      }

      this.ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          this.dispatch(msg.type, msg.data)
        } catch {
          // ignore
        }
      }

      this.ws.onclose = () => {
        this.stopHeartbeat()
        this.emit({ type: 'disconnected' })
      }

      this.ws.onerror = () => {
        reject(new Error('websocket error'))
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

  sendPrewarm(): boolean {
    return this.send('prewarm', {})
  }

  sendPlaybackMark(atMs: number): boolean {
    return this.send('playback_mark', { at_ms: atMs })
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
      case 'proactive_message':
        this.emit({
          type: 'proactive_message',
          message: String(data.message ?? ''),
          animation: typeof data.animation === 'string' ? data.animation : undefined,
          reminderId: typeof data.reminder_id === 'number' ? data.reminder_id : undefined,
        })
        break
      case 'error':
        this.emit({ type: 'error', code: String(data.code), message: String(data.message) })
        break
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
