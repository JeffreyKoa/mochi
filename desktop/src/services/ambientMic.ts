import { PCMCapture, pcmPeakLevel } from '@/services/pcmCapture'
import { getPresenceConfig, getVoiceprintConfig } from '@/config'
import { SoundPresenceTracker, type SoundPresenceState } from '@/services/soundPresence'
import { SpeakerVerifier } from '@/services/speakerVerifier'
import { usePetStore } from '@/stores/petStore'
import { shouldSuppressAudioReturnBubble, tryTriggerAudioPresence } from '@/services/presenceChat'

const SAMPLE_RATE = 16000

let capture: PCMCapture | null = null
let timer: ReturnType<typeof setInterval> | null = null
let ring = new Float32Array(0)
let running = false
let pausedForTalk = false
let ownerEmbedding: Float32Array | null = null

const sharedVerifier = new SpeakerVerifier()

export const ambientPresence = new SoundPresenceTracker(() => getPresenceConfig())

function appendRing(samples: Float32Array, maxSeconds: number) {
  const maxSamples = Math.floor(maxSeconds * SAMPLE_RATE)
  if (ring.length + samples.length <= maxSamples) {
    const merged = new Float32Array(ring.length + samples.length)
    merged.set(ring, 0)
    merged.set(samples, ring.length)
    ring = merged
    return
  }
  const merged = new Float32Array(maxSamples)
  const combined = new Float32Array(ring.length + samples.length)
  combined.set(ring, 0)
  combined.set(samples, ring.length)
  merged.set(combined.subarray(combined.length - maxSamples))
  ring = merged
}

function ringToBuffer(maxSeconds: number): ArrayBuffer {
  const take = Math.min(ring.length, Math.floor(maxSeconds * SAMPLE_RATE))
  const slice = ring.subarray(ring.length - take)
  const buf = new ArrayBuffer(slice.length * 2)
  const view = new DataView(buf)
  for (let i = 0; i < slice.length; i++) {
    const s = Math.max(-1, Math.min(1, slice[i]))
    view.setInt16(i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true)
  }
  return buf
}

function applyPresenceAnimation(next: SoundPresenceState, prev: SoundPresenceState) {
  const pet = usePetStore()
  if (pet.isChatOpen) return
  if (next === 'away' && prev !== 'away') {
    pet.setAnimation('sleep')
  } else if (next === 'owner_present' && prev === 'away') {
    pet.setAnimation('idle')
    if (!shouldSuppressAudioReturnBubble()) {
      pet.showSpeechBubble('你回来了~', 2500)
    }
    void tryTriggerAudioPresence(prev, next)
  } else if (next !== 'away' && prev === 'away') {
    pet.setAnimation('idle')
  }
}

export function initAmbientPresenceHandlers() {
  ambientPresence.setOnChange((next, prev) => {
    usePetStore().ownerPresence = next
    applyPresenceAnimation(next, prev)
  })
}

async function runTick() {
  if (!running || pausedForTalk || ring.length === 0) return
  const vp = getVoiceprintConfig()
  const buf = ringToBuffer(vp.wakeProbeSec)
  await ambientPresence.tickFromChunk(buf, {
    verifier: sharedVerifier,
    ownerEmbedding,
    voiceprintThreshold: vp.threshold,
    wakeProbeSec: vp.wakeProbeSec,
  })
}

async function startCaptureLoop() {
  if (!capture) capture = new PCMCapture()
  if (capture.isActive) return

  await capture.start((pcm) => {
    if (pausedForTalk) return
    const view = new DataView(pcm)
    const floats = new Float32Array(view.byteLength / 2)
    for (let i = 0; i < floats.length; i++) {
      floats[i] = view.getInt16(i * 2, true) / 32768
    }
    appendRing(floats, 4)
    void pcmPeakLevel(pcm)
  })

  const cfg = getPresenceConfig()
  if (timer) clearInterval(timer)
  timer = setInterval(() => void runTick(), cfg.ambientIntervalMs)
  void runTick()
}

async function stopCaptureLoop() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  if (capture?.isActive) {
    await capture.stop()
  }
  ring = new Float32Array(0)
}

export async function setAmbientOwnerEmbedding(embedding: Float32Array | null) {
  ownerEmbedding = embedding
}

export async function bootstrapAmbientPresence(embedding: Float32Array | null) {
  const cfg = getPresenceConfig()
  if (!cfg.enabled) return

  ownerEmbedding = embedding
  initAmbientPresenceHandlers()
  await sharedVerifier.init().catch(() => {})
  running = true
  pausedForTalk = false
  await startCaptureLoop()
}

export async function pauseAmbientMicForTalk() {
  pausedForTalk = true
  ambientPresence.setOwnerSpeaking(true)
  await stopCaptureLoop()
}

export async function resumeAmbientMicAfterTalk(embedding: Float32Array | null) {
  ownerEmbedding = embedding
  pausedForTalk = false
  ambientPresence.setOwnerSpeaking(false)
  if (!running || !getPresenceConfig().enabled) return
  await startCaptureLoop()
}

export async function stopAmbientMic() {
  running = false
  pausedForTalk = false
  await stopCaptureLoop()
  capture = null
  ambientPresence.destroy()
}

export function getAmbientPresenceSnapshot() {
  return ambientPresence.getSnapshot()
}

export function getSharedSpeakerVerifier(): SpeakerVerifier {
  return sharedVerifier
}
