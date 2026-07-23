import { getWSUrl } from './api'

type MessageHandler = (data: unknown) => void

export class WSManager {
  private ws: WebSocket | null = null
  private handlers = new Map<string, Set<MessageHandler>>()
  private reconnectAttempts = 0
  private maxReconnect = 10
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null

  connect() {
    const url = getWSUrl()
    if (!url.includes('token=') || url.endsWith('token=')) return
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }

    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempts = 0
      this.startHeartbeat()
    }

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        this.dispatch(msg.type, msg.data)
      } catch {
        // ignore
      }
    }

    this.ws.onclose = () => {
      this.stopHeartbeat()
      this.attemptReconnect()
    }
  }

  disconnect() {
    this.stopHeartbeat()
    this.ws?.close()
    this.ws = null
  }

  on(type: string, handler: MessageHandler) {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set())
    this.handlers.get(type)!.add(handler)
  }

  off(type: string, handler: MessageHandler) {
    this.handlers.get(type)?.delete(handler)
  }

  send(type: string, data: unknown) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, data, timestamp: Date.now() }))
    }
  }

  private dispatch(type: string, data: unknown) {
    this.handlers.get(type)?.forEach((h) => h(data))
  }

  private startHeartbeat() {
    this.heartbeatTimer = setInterval(() => {
      this.send('heartbeat', { ts: Date.now() })
    }, 30000)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) clearInterval(this.heartbeatTimer)
  }

  private attemptReconnect() {
    if (this.reconnectAttempts >= this.maxReconnect) return
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000)
    this.reconnectAttempts++
    setTimeout(() => this.connect(), delay)
  }
}

export const wsManager = new WSManager()
