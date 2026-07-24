export interface LocalSTTCallbacks {
  onPartial: (text: string) => void
  onFinal: (text: string) => void
  onError?: (message: string) => void
}

type SpeechRecognitionCtor = new () => SpeechRecognition

function getSpeechRecognitionCtor(): SpeechRecognitionCtor | null {
  if (typeof window === 'undefined') return null
  const w = window as Window & {
    SpeechRecognition?: SpeechRecognitionCtor
    webkitSpeechRecognition?: SpeechRecognitionCtor
  }
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null
}

export function isLocalSttSupported(): boolean {
  return getSpeechRecognitionCtor() != null
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
