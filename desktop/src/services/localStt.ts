export interface LocalSTTCallbacks {
  onPartial: (text: string) => void
  onFinal: (text: string) => void
  onError?: (message: string) => void
}

import { isTauri } from '@/services/chatWindow'

type SpeechRecognitionCtor = new () => SpeechRecognition

function getSpeechRecognitionCtor(): SpeechRecognitionCtor | null {
  if (typeof window === 'undefined') return null
  const w = window as Window & {
    SpeechRecognition?: SpeechRecognitionCtor
    webkitSpeechRecognition?: SpeechRecognitionCtor
  }
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null
}

export function isWebSpeechSttSupported(): boolean {
  return getSpeechRecognitionCtor() != null
}

/** 同步：是否可能存在任一本地 STT（Web Speech；X-ASR 需异步 probe）。 */
export function isLocalSttSupported(): boolean {
  return isWebSpeechSttSupported()
}

export type LocalSttBackend = 'xasr' | 'webspeech'

/** 按配置解析本地 STT 后端：X-ASR 优先；Tauri 下不回退 Web Speech（WebView2 不可靠）。 */
export async function resolveLocalSttBackend(cfg: {
  xasr: { enabled: boolean; wsUrl: string }
}): Promise<LocalSttBackend | null> {
  if (cfg.xasr.enabled) {
    const { probeXAsrServer } = await import('@/services/xAsrClient')
    const attempts = isTauri() ? 8 : 2
    for (let attempt = 0; attempt < attempts; attempt++) {
      const timeout = attempt === 0 ? 4000 : 6000
      if (await probeXAsrServer(cfg.xasr.wsUrl, timeout)) {
        return 'xasr'
      }
      if (attempt < attempts - 1) {
        await new Promise((r) => setTimeout(r, 800))
      }
    }
    if (isTauri()) {
      return null
    }
  }
  if (isWebSpeechSttSupported()) {
    return 'webspeech'
  }
  return null
}

/** Device-native STT via Web Speech API (OpenClaw Native Talk style). */
export class LocalSTT {
  private rec: SpeechRecognition | null = null
  private running = false

  start(callbacks: LocalSTTCallbacks, lang = 'zh-CN') {
    const SR = getSpeechRecognitionCtor()
    if (!SR) {
      throw new Error('SpeechRecognition not supported')
    }

    this.stop()
    const rec = new SR()
    rec.lang = lang
    rec.continuous = true
    rec.interimResults = true

    rec.onresult = (event: SpeechRecognitionEvent) => {
      let interim = ''
      let finalText = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i]
        const transcript = result[0]?.transcript ?? ''
        if (result.isFinal) {
          finalText += transcript
        } else {
          interim += transcript
        }
      }
      const partial = interim.trim()
      if (partial) callbacks.onPartial(partial)
      const final = finalText.trim()
      if (final) callbacks.onFinal(final)
    }

    rec.onerror = (event: SpeechRecognitionErrorEvent) => {
      if (event.error === 'no-speech' || event.error === 'aborted') return
      callbacks.onError?.(event.error || 'speech recognition error')
    }

    rec.onend = () => {
      if (!this.running) return
      try {
        rec.start()
      } catch {
        // ignore restart race
      }
    }

    this.rec = rec
    this.running = true
    rec.start()
  }

  stop() {
    this.running = false
    if (!this.rec) return
    try {
      this.rec.onend = null
      this.rec.stop()
    } catch {
      // ignore
    }
    this.rec = null
  }

  get isRunning() {
    return this.running
  }
}
