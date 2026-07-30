import type { RealtimePresenceConfig } from '@/config'
import { pcmPeakLevel } from '@/services/pcmCapture'
import { pcmToFloat } from '@/services/sileroSpeechVad'
import { SoundEventClassifier } from '@/services/soundEventClassifier'
import type { SpeakerVerifier } from '@/services/speakerVerifier'

export type SoundPresenceState =
  | 'away'
  | 'nearby'
  | 'human_voice'
  | 'owner_present'
  | 'owner_speaking'

export interface SoundPresenceSnapshot {
  sound_presence: SoundPresenceState
  last_human_voice_sec: number | null
  last_owner_voice_sec: number | null
}

export type PresenceChangeHandler = (
  next: SoundPresenceState,
  prev: SoundPresenceState,
) => void

export class SoundPresenceTracker {
  private state: SoundPresenceState = 'away'
  private lastHumanVoiceAt = 0
  private lastOwnerVoiceAt = 0
  private lastEnergyAt = 0
  private ownerSpeaking = false
  private onChange: PresenceChangeHandler | null = null
  private classifier: SoundEventClassifier | null = null
  private classifierInit = false

  constructor(private cfg: () => RealtimePresenceConfig) {}

  setOnChange(handler: PresenceChangeHandler | null) {
    this.onChange = handler
  }

  getState(): SoundPresenceState {
    return this.state
  }

  getSnapshot(): SoundPresenceSnapshot {
    const now = Date.now()
    return {
      sound_presence: this.state,
      last_human_voice_sec: this.lastHumanVoiceAt
        ? Math.floor((now - this.lastHumanVoiceAt) / 1000)
        : null,
      last_owner_voice_sec: this.lastOwnerVoiceAt
        ? Math.floor((now - this.lastOwnerVoiceAt) / 1000)
        : null,
    }
  }

  setOwnerSpeaking(active: boolean) {
    this.ownerSpeaking = active
    if (active) {
      this.transition('owner_speaking')
      this.lastOwnerVoiceAt = Date.now()
      this.lastHumanVoiceAt = Date.now()
      return
    }
    this.recomputeFromTimestamps()
  }

  noteNearby() {
    if (this.ownerSpeaking) return
    this.transition('nearby')
  }

  noteHumanVoice() {
    if (this.ownerSpeaking) return
    this.lastHumanVoiceAt = Date.now()
    this.transition('human_voice')
  }

  noteOwnerPresent() {
    if (this.ownerSpeaking) return
    this.lastOwnerVoiceAt = Date.now()
    this.lastHumanVoiceAt = Date.now()
    this.transition('owner_present')
  }

  private recomputeFromTimestamps() {
    if (this.ownerSpeaking) return
    const cfg = this.cfg()
    const now = Date.now()
    const ownerTtlMs = cfg.ownerPresenceTtlSec * 1000
    const awayMs = cfg.awayTimeoutSec * 1000

    if (this.lastOwnerVoiceAt && now - this.lastOwnerVoiceAt <= ownerTtlMs) {
      this.transition('owner_present')
      return
    }
    if (this.lastHumanVoiceAt && now - this.lastHumanVoiceAt <= awayMs) {
      this.transition('human_voice')
      return
    }
    if (this.lastEnergyAt && now - this.lastEnergyAt <= awayMs) {
      this.transition('nearby')
      return
    }
    this.transition('away')
  }

  private transition(next: SoundPresenceState) {
    if (this.state === next) return
    const prev = this.state
    this.state = next
    this.onChange?.(next, prev)
  }

  private async ensureClassifier(): Promise<SoundEventClassifier | null> {
    if (!this.cfg().enabled) return null
    if (!this.classifier) {
      this.classifier = new SoundEventClassifier()
    }
    if (!this.classifierInit) {
      await this.classifier.init()
      this.classifierInit = true
    }
    return this.classifier.available ? this.classifier : null
  }

  /** Ambient tick from raw PCM chunk (ArrayBuffer, 16 kHz mono). */
  async tickFromChunk(
    pcmBuf: ArrayBuffer,
    deps: {
      verifier: SpeakerVerifier
      ownerEmbedding: Float32Array | null
      voiceprintThreshold: number
      wakeProbeSec: number
    },
  ) {
    const cfg = this.cfg()
    if (!cfg.enabled || this.ownerSpeaking) return

    const peak = pcmPeakLevel(pcmBuf)
    const now = Date.now()
    if (peak >= cfg.ambientEnergyFloor) {
      this.lastEnergyAt = now
    }

    const pcm = pcmToFloat(pcmBuf)
    const sampleCount = Math.min(pcm.length, Math.floor(deps.wakeProbeSec * 16000))
    const window = pcm.subarray(Math.max(0, pcm.length - sampleCount))

    if (peak < cfg.ambientEnergyFloor) {
      this.recomputeFromTimestamps()
      return
    }

    const classifier = await this.ensureClassifier()
    if (classifier) {
      const result = await classifier.classify(window)
      if (!result || result.speechScore < cfg.speechThreshold) {
        this.noteNearby()
        return
      }
    }

    this.noteHumanVoice()

    if (
      deps.ownerEmbedding &&
      deps.ownerEmbedding.length > 0 &&
      deps.verifier.available
    ) {
      try {
        const verify = await deps.verifier.verify(
          window,
          deps.ownerEmbedding,
          deps.voiceprintThreshold,
        )
        if (verify?.match) {
          this.noteOwnerPresent()
        }
      } catch {
        // fail-closed for presence: stay at human_voice
      }
    }

    const awayMs = cfg.awayTimeoutSec * 1000
    if (this.lastHumanVoiceAt && now - this.lastHumanVoiceAt > awayMs) {
      this.transition('away')
    }
  }

  destroy() {
    this.classifier?.destroy()
    this.classifier = null
    this.classifierInit = false
    this.onChange = null
  }
}
