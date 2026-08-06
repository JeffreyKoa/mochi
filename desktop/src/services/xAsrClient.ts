/** X-ASR sherpa_streaming_server WebSocket 协议（客户端优先 POC）。 */

export type XAsrServerMessage =
  | { type: 'started'; sample_rate: number }
  | { type: 'partial'; text: string }
  | { type: 'final'; text: string; first_partial_latency?: number }
  | { type: 'error'; text: string }
  | { type: 'reset_ok'; version?: string }
  | { type: 'pong'; version?: string }

export interface XAsrClientHandlers {
  onPartial?: (text: string) => void
  onError?: (message: string) => void
}

const DEFAULT_CONNECT_MS = 3000
const DEFAULT_FINAL_MS = 5000

export class XAsrClient {
  private ws: WebSocket | null = null
  private url = ''
  private sessionActive = false
  private handlers: XAsrClientHandlers = {}
  private startWaiter: { resolve: () => void; reject: (e: Error) => void } | null = null
  private finalWaiter: { resolve: (text: string) => void; reject: (e: Error) => void } | null = null
  private finalTimer: ReturnType<typeof setTimeout> | null = null
  /** ping 等待 pong（走 handleMessage，避免 WebView2 下 addEventListener 不可靠）。 */
  private pingWaiter: (() => void) | null = null
  /** 最近一次 pong 携带的 sidecar 版本（health 校验用）。 */
  lastPongVersion: string | null = null

  setHandlers(handlers: XAsrClientHandlers) {
    this.handlers = handlers
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  get hasSession(): boolean {
    return this.sessionActive
  }

  /** 建立 WebSocket；失败返回 false。 */
  async connect(url: string, timeoutMs = DEFAULT_CONNECT_MS): Promise<boolean> {
    if (this.connected && this.url === url) return true
    this.close()

    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        cleanup()
        // 超时须关闭 socket，否则 sidecar 连接泄漏会导致握手超时、探测 offline
        try {
          ws.onclose = null
          ws.close()
        } catch {
          // ignore
        }
        resolve(false)
      }, timeoutMs)

      const cleanup = () => {
        clearTimeout(timer)
        ws.onopen = null
        ws.onerror = null
      }

      let ws: WebSocket
      try {
        ws = new WebSocket(url)
        ws.binaryType = 'arraybuffer'
      } catch {
        clearTimeout(timer)
        resolve(false)
        return
      }

      ws.onopen = () => {
        cleanup()
        this.ws = ws
        this.url = url
        ws.onmessage = (ev) => this.handleMessage(ev)
        ws.onclose = () => {
          this.ws = null
          this.sessionActive = false
          this.rejectWaiters(new Error('x-asr connection closed'))
        }
        ws.onerror = () => {
          this.handlers.onError?.('x-asr websocket error')
        }
        resolve(true)
      }

      ws.onerror = () => {
        cleanup()
        ws.close()
        resolve(false)
      }
    })
  }

  /** 健康检查：ping / pong。 */
  async ping(timeoutMs = 2000): Promise<boolean> {
    if (!this.connected) return false
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.pingWaiter = null
        resolve(false)
      }, timeoutMs)
      this.pingWaiter = () => {
        clearTimeout(timer)
        this.pingWaiter = null
        resolve(true)
      }
      this.sendJson({ type: 'ping' })
    })
  }

  /** 开启一轮识别会话。 */
  async startSession(sampleRate = 16000): Promise<void> {
    if (!this.connected) {
      throw new Error('x-asr not connected')
    }
    if (this.sessionActive) {
      await this.resetSession()
    }

    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.startWaiter = null
        reject(new Error('x-asr start timeout'))
      }, DEFAULT_CONNECT_MS)

      this.startWaiter = {
        resolve: () => {
          clearTimeout(timer)
          this.startWaiter = null
          this.sessionActive = true
          resolve()
        },
        reject: (e) => {
          clearTimeout(timer)
          this.startWaiter = null
          reject(e)
        },
      }

      this.sendJson({ type: 'start', sample_rate: sampleRate })
    })
  }

  /** 发送 int16 mono PCM（ArrayBuffer 字节长度须为 2 的倍数）。 */
  sendPcm(pcm: ArrayBuffer): void {
    if (!this.sessionActive || !this.connected) return
    if (pcm.byteLength === 0) return
    this.ws?.send(pcm)
  }

  /** 结束当前会话并等待 final 文本。 */
  async endSession(timeoutMs = DEFAULT_FINAL_MS): Promise<string> {
    if (!this.sessionActive || !this.connected) {
      return ''
    }

    return new Promise((resolve, reject) => {
      this.finalTimer = setTimeout(() => {
        this.finalWaiter = null
        this.sessionActive = false
        reject(new Error('x-asr final timeout'))
      }, timeoutMs)

      this.finalWaiter = {
        resolve: (text) => {
          if (this.finalTimer) clearTimeout(this.finalTimer)
          this.finalTimer = null
          this.finalWaiter = null
          this.sessionActive = false
          resolve(text)
        },
        reject: (e) => {
          if (this.finalTimer) clearTimeout(this.finalTimer)
          this.finalTimer = null
          this.finalWaiter = null
          this.sessionActive = false
          reject(e)
        },
      }

      this.sendJson({ type: 'end' })
    })
  }

  /** 丢弃当前会话状态（名字探针取消等）。 */
  async resetSession(): Promise<void> {
    if (!this.connected) return
    this.sessionActive = false
    this.rejectWaiters(new Error('x-asr session reset'))
    this.sendJson({ type: 'reset' })
  }

  close() {
    this.sessionActive = false
    this.pingWaiter = null
    this.rejectWaiters(new Error('x-asr client closed'))
    if (this.finalTimer) {
      clearTimeout(this.finalTimer)
      this.finalTimer = null
    }
    if (this.ws) {
      try {
        this.ws.onclose = null
        this.ws.close()
      } catch {
        // ignore
      }
    }
    this.ws = null
  }

  private rejectWaiters(err: Error) {
    this.startWaiter?.reject(err)
    this.startWaiter = null
    this.finalWaiter?.reject(err)
    this.finalWaiter = null
  }

  private sendJson(payload: Record<string, unknown>) {
    this.ws?.send(JSON.stringify(payload))
  }

  private handleMessage(ev: MessageEvent) {
    if (typeof ev.data !== 'string') return
    let msg: XAsrServerMessage
    try {
      msg = JSON.parse(ev.data) as XAsrServerMessage
    } catch {
      return
    }

    switch (msg.type) {
      case 'started':
        this.startWaiter?.resolve()
        break
      case 'partial':
        if (msg.text) this.handlers.onPartial?.(msg.text)
        break
      case 'final':
        this.finalWaiter?.resolve(msg.text ?? '')
        break
      case 'error':
        this.handlers.onError?.(msg.text || 'x-asr error')
        this.finalWaiter?.reject(new Error(msg.text || 'x-asr error'))
        this.startWaiter?.reject(new Error(msg.text || 'x-asr error'))
        break
      case 'reset_ok':
        break
      case 'pong':
        this.lastPongVersion = typeof msg.version === 'string' ? msg.version : null
        this.pingWaiter?.()
        break
      default:
        break
    }
  }
}

/** 与 sidecar 内置版本对齐。 */
export const EXPECTED_XASR_VERSION = '1'

/** 探测 X-ASR sidecar 是否在线（connect + ping）；始终关闭探测用连接。 */
export async function probeXAsrServer(
  url: string,
  timeoutMs = 2500,
): Promise<boolean> {
  const client = new XAsrClient()
  let versionMismatch = false
  try {
    const connected = await client.connect(url, timeoutMs)
    if (!connected) {
      return false
    }
    const pong = await client.ping(Math.min(timeoutMs, 2000))
    versionMismatch = client.lastPongVersion != null && client.lastPongVersion !== EXPECTED_XASR_VERSION
    if (versionMismatch && import.meta.env.DEV) {
      console.warn(
        '[x-asr] sidecar version mismatch: got %s expected %s',
        client.lastPongVersion,
        EXPECTED_XASR_VERSION,
      )
    }
    return pong || connected
  } finally {
    client.close()
  }
}
