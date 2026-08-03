import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  PCMCapture,
  arrayBufferToBase64,
  pcmPeakLevel,
  amplifyPCM,
  float32ToPcm16LE,
} from '@/services/pcmCapture'
import { realtimeSession, type RealtimeEvent } from '@/services/realtimeSession'
import { usePetStore } from '@/stores/petStore'
import { TTSAudioQueue, isOpusDecodeSupported } from '@/services/ttsAudioPlayer'
import { HybridSpeechVad, pcmToFloat, type VADEvent } from '@/services/sileroSpeechVad'
import { LocalSTT, isLocalSttSupported } from '@/services/localStt'
import { SpeakerVerifier } from '@/services/speakerVerifier'
import { SoundEventClassifier } from '@/services/soundEventClassifier'
import { getRealtimeConfig, getVoiceprintConfig, getFaceprintConfig, getPresenceConfig, getClientConfig, initClientConfig, resolveSttMode } from '@/config'
import { micPermissionDeniedMessage } from '@/utils/micPermission'
import { stripMoodTags } from '@/utils/stripMoodTags'
import {
  captureOwnerFaceJPEG,
  isVisionCaptureEnabled,
  jpegToBase64,
  startVisionSession,
  stopVisionSession,
  visionSession,
  type VisionFrameReason,
} from '@/services/visionCapture'
import {
  bootstrapAmbientPresence as startAmbientPresenceService,
  pauseAmbientMicForTalk,
  resumeAmbientMicAfterTalk,
  ambientPresence,
} from '@/services/ambientMic'
import { handleProactiveMessage } from '@/services/proactiveHandler'
import { streamChatMessage, getVoiceprintStatus, getFaceprintStatus } from '@/services/api'
import {
  type VoiceOwner,
  getStoredVoiceOwner,
} from '@/services/voiceSessionOwner'
import { isTauri } from '@/services/chatWindow'
import { looksLikeObjectQuery } from '@/services/visionHeuristic'
import { identityGate } from '@/services/identityGate'
import { FaceVerifier, FACE_OWNER_BOOST_SCORE } from '@/services/faceVerifier'
import {
  evaluateTurnEnd,
  PAUSE_PROBE_MS,
} from '@/services/turnEndArbiter'

/**
 * Turn phases — like talking to a person:
 * resting: mic monitors locally, Mochi sleeps, no upload
 * user_speaking: owner voice detected, recording & uploading
 * processing / agent_speaking: Mochi thinks & replies
 */
type TurnPhase = 'idle' | 'resting' | 'user_speaking' | 'processing' | 'agent_speaking'

export type WakeListeningFailure =
  | 'not_ready'
  | 'voiceprint_missing'
  | 'disconnected'
  | 'not_owner'
  | 'not_speech'

export type WakeListeningResult =
  | { ok: true }
  | { ok: false; reason: WakeListeningFailure }

/** wakeListening 选项：manual=true 表示用户主动点击，跳过唤醒前声纹探测。 */
export interface WakeListeningOptions {
  manual?: boolean
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  source?: 'voice' | 'text'
  createdAt?: string | number
  dismissed?: boolean
  dismissReason?: string
}

const WAKE_CONFIRM_MS = 450
/** 声纹得分低于此值才口语拒答「不是主人」；介于其间视为没听清，静默忽略。 */
const NON_OWNER_REPLY_SCORE_MAX = 0.28
const TTS_WATCHDOG_MS = 45000
const TEXT_TURN_ACK_MS = 6000
const MAX_UTTERANCE_MS = 10000
const PCM_RING_CHUNKS = 200 // ~4 s @ 20 ms/chunk
const PCM_RING_SAMPLES = PCM_RING_CHUNKS * 320 // 16 kHz mono
const HEARD_BUBBLE_GRACE_MS = 3000

interface RuntimeParams {
  silenceMs: number
  bargeInPeak: number
  bargeInMs: number
  echoGuardMs: number
  wakePeak: number
  speechPeak: number
  tailSpeechPeak: number
  endpointDebounceMs: number
  minEndpointChars: number
  endpointingEnabled: boolean
}

function defaultRuntimeParams(): RuntimeParams {
  const rt = getRealtimeConfig()
  return {
    silenceMs: rt.vad.silenceMs,
    bargeInPeak: rt.bargeIn.peakThreshold,
    bargeInMs: rt.bargeIn.bargeInMs,
    echoGuardMs: rt.bargeIn.echoGuardMs,
    wakePeak: rt.vad.wakePeak,
    speechPeak: rt.vad.energyPeak,
    tailSpeechPeak: rt.vad.tailSpeechPeak,
    endpointDebounceMs: 1200,
    minEndpointChars: 8,
    endpointingEnabled: rt.vad.endpointingEnabled,
  }
}

export const useRealtimeStore = defineStore('realtime', () => {
  const connected = ref(false)
  const talking = ref(false)
  const resting = ref(true)
  const statusText = ref('')
  const partialText = ref('')
  const replyText = ref('')
  const messages = ref<ChatMessage[]>([])
  const sessionId = ref('')
  const micLevel = ref(0)
  const chunksSent = ref(0)
  const processingRef = ref(false)

  const userSpeaking = computed(
    () => talking.value && !resting.value && !processingRef.value,
  )

  const capture = new PCMCapture()
  const ttsPlayer = new TTSAudioQueue()
  let localStt: LocalSTT | null = null
  let effectiveSttMode: 'cloud' | 'local' = 'cloud'
  let params = defaultRuntimeParams()
  let recording = false
  let phase: TurnPhase = 'idle'
  let uploadSeq = 0
  let chunksSentCount = 0
  let peakSeen = 0
  let heardSpeech = false
  let lastSpeechAt = 0
  let utteranceStartedAt = 0
  let silenceTimer: ReturnType<typeof setInterval> | null = null
  let ttsWatchdog: ReturnType<typeof setTimeout> | null = null
  let unsub: (() => void) | null = null
  let speechVad: HybridSpeechVad | null = null
  let submitLock = false
  let ttsStartedAt = 0
  let bargeAccumMs = 0
  let wakeAccumMs = 0
  let textSending = false
  let pendingTextTurn: string | null = null
  let textViaRest = false
  let turnAckWaiter: { resolve: (ok: boolean) => void; timer: ReturnType<typeof setTimeout> } | null =
    null
  let turnStartAt = 0
  let playbackMarked = false
  let lastEndpointAt = 0
  let lastTurnMetrics: import('@/services/realtimeSession').TurnMetrics | null = null
  /** Which window may hold /ws/voice: pet | chat | inline (browser single-window). */
  let voiceWindow: VoiceOwner | 'inline' = 'inline'
  let connectFlight: Promise<void> | null = null
  let intentionalDisconnect = false
  let reconnecting = false

  function detachHandler() {
    unsub?.()
    unsub = null
  }

  function setVoiceWindow(window: VoiceOwner | 'inline') {
    voiceWindow = window
  }

  async function shouldOwnVoice(): Promise<boolean> {
    if (voiceWindow === 'inline') return true
    if (!isTauri()) return true
    const owner = getStoredVoiceOwner()
    if (voiceWindow === 'chat') return owner === 'chat'
    // 设置与聊天共用 chat 弹窗；仅以 voice-owner 为准，避免设置页误拦宠物语音
    if (voiceWindow === 'pet') {
      return owner === null || owner === 'pet'
    }
    return false
  }

  async function yieldVoiceConnection() {
    ttsPlayer.stop()
    if (recording || talking.value) {
      await endConversation()
    } else {
      disconnect()
    }
  }

  async function connectIfOwner() {
    if (!(await shouldOwnVoice())) return
    await connect()
  }

  const speakerVerifier = new SpeakerVerifier()
  const faceVerifier = new FaceVerifier()
  const soundClassifier = new SoundEventClassifier()
  let ownerEmbedding: Float32Array | null = null
  let ownerFaceEmbedding: Float32Array | null = null
  let wakeProbeInFlight = false
  /** 最近一次唤醒 probe 的声纹得分（供拒答策略判断）。 */
  let lastWakeProbeScore: number | null = null
  /** 对话中声纹 stream_check 定时器。 */
  let streamCheckTimer: ReturnType<typeof setInterval> | null = null
  let streamCheckGraceTimer: ReturnType<typeof setTimeout> | null = null
  /** P2：对话中人脸 probe 定时器。 */
  let faceCheckTimer: ReturnType<typeof setInterval> | null = null
  /** 最近一次人脸 probe 结果（供 identity gate 融合）。 */
  let lastFaceProbe: { match: boolean; score: number; detected: boolean } | null = null
  /** P2：本 turn 是否已发过 object_refresh 帧（partial 中途触发后 submit 仍可再发一次）。 */
  let objectRefreshCountThisTurn = 0
  /** 本 turn 是否已发 speech_start 帧（防 VAD 重复触发）。 */
  let speechStartFrameSent = false
  /** 主人 turn 内锁定 PCM 上传，stream_check 不误拦导致 ASR 空。 */
  let ownerTurnUploadLock = false
  /** 本 turn 是否已开始上传（首包语音后连续上传，保证流式 ASR 不断流）。 */
  let turnUploadStarted = false
  /** ASR partial 最近一次更新时间（眼耳协同：partial 仍在变则不提交）。 */
  let partialUpdatedAt = 0
  /** 多模态仲裁：延长等待截止时间（主人可能还在组织语言）。 */
  let thinkingHoldUntil = 0
  /** 本 turn 是否已做过 pause_probe（静音≥3s 眼抓拍）。 */
  let pauseProbeDone = false
  let pauseProbeInFlight = false
  /** pause_probe 之后用户是否已继续说话（区分句中停顿 vs 句末结束）。 */
  let resumedAfterPauseProbe = false
  /** 举物补拍前留给用户对准摄像头的延迟（ms）。 */
  const OBJECT_FRAME_HOLD_MS = 400
  let speechEndSubmitTimer: ReturnType<typeof setTimeout> | null = null
  const pcmRing = new Float32Array(PCM_RING_SAMPLES)
  let pcmRingWrite = 0

  function pushPcmRing(samples: Float32Array) {
    for (let i = 0; i < samples.length; i++) {
      pcmRing[pcmRingWrite] = samples[i]
      pcmRingWrite = (pcmRingWrite + 1) % PCM_RING_SAMPLES
    }
  }

  function snapshotPcmRing(): Float32Array {
    const out = new Float32Array(PCM_RING_SAMPLES)
    const tail = pcmRing.subarray(pcmRingWrite)
    const head = pcmRing.subarray(0, pcmRingWrite)
    out.set(tail, 0)
    out.set(head, tail.length)
    return out
  }

  function resetPcmRing() {
    pcmRing.fill(0)
    pcmRingWrite = 0
  }

  async function loadOwnerVoiceprint() {
    ownerEmbedding = null
    try {
      const status = await getVoiceprintStatus()
      if (status.enrolled && status.embedding?.length) {
        ownerEmbedding = new Float32Array(status.embedding)
      }
    } catch {
      ownerEmbedding = null
    }
  }

  async function loadOwnerFaceprint() {
    ownerFaceEmbedding = null
    try {
      const status = await getFaceprintStatus()
      if (status.enrolled && status.embedding?.length) {
        ownerFaceEmbedding = new Float32Array(status.embedding)
      }
    } catch {
      ownerFaceEmbedding = null
    }
  }

  function faceprintReady(): boolean {
    const fp = getFaceprintConfig()
    if (!fp.enabled) return false
    if (!fp.required) return faceVerifier.available && !!ownerFaceEmbedding?.length
    return faceVerifier.available && !!ownerFaceEmbedding?.length
  }

  async function probeFaceFromJPEG(jpeg: ArrayBuffer): Promise<{ match: boolean; score: number; detected: boolean } | null> {
    const fp = getFaceprintConfig()
    if (!fp.enabled || !faceVerifier.available || !ownerFaceEmbedding?.length) return null
    try {
      const result = await faceVerifier.verify(jpeg, ownerFaceEmbedding, fp.matchThreshold)
      if (!result) return null
      if (result.detected) {
        lastFaceProbe = result
        identityGate.noteFaceResult(result)
      }
      return result
    } catch {
      return null
    }
  }

  /** 唤醒前快拍验脸：声纹失败时若人脸高分仍视为主人。 */
  async function quickFaceProbeForWake(): Promise<{ match: boolean; score: number; detected: boolean } | null> {
    if (!faceprintReady()) return null
    const buf = visionSession.isActive()
      ? await visionSession.grabSnapshot()
      : await captureOwnerFaceJPEG()
    if (!buf) return null
    return probeFaceFromJPEG(buf)
  }

  /** 人脸是否足以否决 non_owner 拒答 / 辅助唤醒。 */
  function faceSupportsOwner(face: { match: boolean; score: number; detected: boolean } | null): boolean {
    if (!face) return false
    if (face.score >= FACE_OWNER_BOOST_SCORE) return true
    return face.detected && face.match
  }

  function snapshotRecentPcm(maxSeconds?: number): Float32Array {
    const sec = maxSeconds ?? getVoiceprintConfig().wakeProbeSec
    const maxSamples = Math.floor(sec * 16000)
    const full = snapshotPcmRing()
    if (full.length <= maxSamples) return full
    return full.subarray(full.length - maxSamples)
  }

  async function verifyOwnerVoice(
    pcm: Float32Array,
    useMaxScore = true,
  ): Promise<{ match: boolean; score: number } | null> {
    const vp = getVoiceprintConfig()
    if (!speakerVerifier.available || !ownerEmbedding?.length) return null
    try {
      if (useMaxScore) {
        return await speakerVerifier.verifyMaxScore(
          pcm,
          ownerEmbedding,
          vp.threshold,
          1.5,
          0.5,
        )
      }
      return await speakerVerifier.verify(pcm, ownerEmbedding, vp.threshold)
    } catch {
      return null
    }
  }

  function voiceprintReady(): boolean {
    const vp = getVoiceprintConfig()
    if (!vp.required) return true
    return speakerVerifier.available && !!ownerEmbedding?.length
  }

  function stopStreamCheck() {
    if (streamCheckGraceTimer) {
      clearTimeout(streamCheckGraceTimer)
      streamCheckGraceTimer = null
    }
    if (streamCheckTimer) {
      clearInterval(streamCheckTimer)
      streamCheckTimer = null
    }
    stopFaceCheck()
  }

  function stopFaceCheck() {
    if (faceCheckTimer) {
      clearInterval(faceCheckTimer)
      faceCheckTimer = null
    }
  }

  /** P2：周期性从会话摄像头抓拍并验脸。 */
  function startFaceCheck() {
    stopFaceCheck()
    const fp = getFaceprintConfig()
    if (!fp.enabled || !faceprintReady()) return
    const interval = fp.checkIntervalMs || 2000
    faceCheckTimer = setInterval(() => {
      void runFaceCheck()
    }, interval)
  }

  async function runFaceCheck() {
    if (phase !== 'user_speaking' || !recording) return
    if (!visionSession.isActive() && !isVisionCaptureEnabled()) return
    const buf = visionSession.isActive()
      ? await visionSession.grabSnapshot()
      : await captureOwnerFaceJPEG()
    if (!buf) return
    const face = await probeFaceFromJPEG(buf)
    if (import.meta.env.DEV && face) {
      console.debug('[faceprint] periodic_probe match=%s score=%s', face.match, face.score.toFixed(3))
    }
  }

  /** 对话中周期性验声纹（P0 stream_check）+ 人脸融合（P2）。 */
  function startStreamCheck() {
    stopStreamCheck()
    const vp = getVoiceprintConfig()
    if (!vp.required || !voiceprintReady() || effectiveSttMode === 'local') return
    const interval = vp.streamCheckIntervalMs || 500
    // 唤醒宽限期：先上传首包音频，再启动 stream_check（避免首秒误拦）
    const graceMs = 1200
    const graceTimer = setTimeout(() => {
      streamCheckTimer = setInterval(() => {
        void runStreamCheck()
      }, interval)
    }, graceMs)
    // 存 grace timer 以便 stopStreamCheck 清理
    streamCheckGraceTimer = graceTimer
    startFaceCheck()
  }

  async function runStreamCheck() {
    if (phase !== 'user_speaking' || !recording) return
    // 主人已唤醒的 turn 内不做 PCM 门控，避免声纹瞬时偏低导致 ASR 识别为空
    if (ownerTurnUploadLock) return
    const vp = getVoiceprintConfig()
    const fp = getFaceprintConfig()
    const windowSec = Math.min(vp.verifyWindowSec, 2)
    const pcm = snapshotRecentPcm(windowSec)
    const result = await verifyOwnerVoice(pcm, true)
    const faceForGate = lastFaceProbe ?? identityGate.lastFaceResult
    const mode = identityGate.applyIdentityResult(
      result ?? { match: false, score: 0 },
      faceForGate,
      vp,
      fp,
    )
    if (import.meta.env.DEV) {
      console.debug(
        '[identity_gate] stream_check mode=%s voice=%s face=%s',
        mode,
        result?.score?.toFixed(3) ?? 'null',
        lastFaceProbe?.score?.toFixed(3) ?? 'null',
      )
    }
    if (mode === 'foreign_only' && import.meta.env.DEV) {
      console.debug('[identity_gate] stream_check foreign_only (filter only, no TTS)')
    }
  }

  /** 场景①：非主人明确对 Mochi 说话 → 服务端 TTS 拒答（仅高置信非主人）。 */
  async function sendNonOwnerReply(immediate = false, score?: number | null) {
    const vp = getVoiceprintConfig()
    // P2：人脸高分视为主人 → 不因声纹 alone 拒答（必要时即时补拍）
    let face = lastFaceProbe
    if (!faceSupportsOwner(face)) {
      face = await quickFaceProbeForWake()
    }
    if (faceSupportsOwner(face)) {
      if (import.meta.env.DEV) {
        console.debug(
          '[faceprint] skip non_owner_reply face_score=%s',
          face?.score?.toFixed(3) ?? 'null',
        )
      }
      return
    }
    // 得分在阈值附近：可能是主人或杂音，不说「不是主人」
    if (score != null && score >= NON_OWNER_REPLY_SCORE_MAX) {
      if (import.meta.env.DEV) {
        console.debug('[voiceprint] skip non_owner_reply score=%s (borderline)', score.toFixed(3))
      }
      return
    }
    if (!identityGate.shouldTriggerNonOwnerReply(vp, { immediate })) return
    if (!realtimeSession.isOpen()) {
      await connectIfOwner()
      if (!realtimeSession.isOpen()) return
    }
    identityGate.markNonOwnerReplySent()
    realtimeSession.sendNonOwnerTurn()
    usePetStore().showSpeechBubble('你好像不是主人呢…', 3000)

    if (phase === 'user_speaking') {
      stopStreamCheck()
      if (chunksSentCount === 0) {
        realtimeSession.sendUtteranceCancel()
      }
      partialText.value = ''
      heardSpeech = false
      identityGate.resetTurn()
      enterResting()
    }
  }

  function commitHeardText(
    text: string,
    meta?: { dismissed?: boolean; dismissReason?: string },
  ) {
    const trimmed = text.trim()
    if (!trimmed) return
    const last = messages.value[messages.value.length - 1]
    if (last?.role === 'user' && last.content === trimmed) {
      if (meta?.dismissed && !last.dismissed) {
        last.dismissed = true
        last.dismissReason = meta.dismissReason
      }
      return
    }
    messages.value.push({
      role: 'user',
      content: trimmed,
      source: 'voice',
      createdAt: Date.now(),
      dismissed: meta?.dismissed,
      dismissReason: meta?.dismissReason,
    })
    partialText.value = trimmed
    const pet = usePetStore()
    if (!pet.isChatOpen) {
      pet.showVoiceBubble(`"${trimmed}"`)
      pet.releaseVoiceBubble(HEARD_BUBBLE_GRACE_MS)
    }
  }

  async function probeWakeFromResting(): Promise<'ok' | 'not_owner' | 'not_speech' | 'skip'> {
    if (phase !== 'resting' || !recording || wakeProbeInFlight) return 'skip'
    if (!voiceprintReady()) return 'skip'

    wakeProbeInFlight = true
    try {
      const pcm = snapshotRecentPcm(getVoiceprintConfig().wakeProbeSec)
      const presenceCfg = getPresenceConfig()

      if (presenceCfg.enabled && soundClassifier.available) {
        const cls = await soundClassifier.classify(pcm)
        if (cls && cls.speechScore < presenceCfg.speechThreshold) {
          ambientPresence.noteNearby()
          wakeAccumMs = 0
          return 'not_speech'
        }
      }

      const result = await verifyOwnerVoice(pcm, true)
      lastWakeProbeScore = result?.score ?? null
      if (!result?.match) {
        // P2：声纹未过但人脸高分 → 仍唤醒（降误拒）
        const face = await quickFaceProbeForWake()
        if (faceSupportsOwner(face)) {
          if (import.meta.env.DEV) {
            console.debug(
              '[identity_gate] wake face_boost voice=%s face=%s',
              result?.score?.toFixed(3) ?? 'null',
              face?.score?.toFixed(3) ?? 'null',
            )
          }
          identityGate.markOwnerMatch()
          wakeOnSpeech()
          return 'ok'
        }
        if (import.meta.env.DEV) {
          console.debug('[voiceprint] wake rejected score=%s', result?.score?.toFixed(3) ?? 'null')
        }
        identityGate.applyIdentityResult(
          result ?? { match: false, score: 0 },
          lastFaceProbe,
          getVoiceprintConfig(),
          getFaceprintConfig(),
        )
        wakeAccumMs = 0
        return 'not_owner'
      }

      identityGate.markOwnerMatch()
      wakeOnSpeech()
      return 'ok'
    } finally {
      wakeProbeInFlight = false
    }
  }

  async function tryWakeFromResting() {
    const probe = await probeWakeFromResting()
    if (probe === 'ok') return
    // VAD 已报 speech_start 但声纹/分类器误拒：能量明显时仍唤醒（宠物点按场景）
    if (
      (probe === 'not_owner' || probe === 'not_speech') &&
      micLevel.value >= params.wakePeak * 1.5
    ) {
      if (import.meta.env.DEV) {
        console.debug('[voiceprint] vad energy wake fallback peak=%s', micLevel.value.toFixed(3))
      }
      identityGate.markOwnerMatch()
      wakeOnSpeech()
      return
    }
    if (probe === 'not_owner' && import.meta.env.DEV) {
      console.debug('[voiceprint] wake silent reject score=%s', lastWakeProbeScore?.toFixed(3) ?? 'null')
    }
  }

  function resetTurnTiming() {
    turnStartAt = 0
    playbackMarked = false
  }

  function markPlaybackStart() {
    if (playbackMarked || turnStartAt <= 0) return
    playbackMarked = true
    const atMs = Date.now() - turnStartAt
    realtimeSession.sendPlaybackMark(atMs)
  }
  function commitUserMessage(text: string, source: 'voice' | 'text') {
    const trimmed = text.trim()
    if (!trimmed) return
    const last = messages.value[messages.value.length - 1]
    if (last?.role === 'user' && last.content === trimmed) return
    messages.value.push({ role: 'user', content: trimmed, source, createdAt: Date.now() })
  }

  function commitAssistantMessage(text: string) {
    const trimmed = text.trim()
    if (!trimmed) return
    const last = messages.value[messages.value.length - 1]
    if (last?.role === 'assistant' && last.content === trimmed) return
    messages.value.push({ role: 'assistant', content: trimmed, createdAt: Date.now() })
  }

  function loadHistory(history: ChatMessage[]) {
    messages.value = history.map((m) => ({
      role: m.role,
      content: m.content,
      source: m.source,
      createdAt: m.createdAt,
      dismissed: m.dismissed,
      dismissReason: m.dismissReason,
    }))
  }

  function clearTurnAckWait() {
    if (!turnAckWaiter) return
    clearTimeout(turnAckWaiter.timer)
    turnAckWaiter = null
  }

  function beginTurnAckWait(ms: number): Promise<boolean> {
    clearTurnAckWait()
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        turnAckWaiter = null
        resolve(false)
      }, ms)
      turnAckWaiter = { resolve, timer }
    })
  }

  function signalTurnAck() {
    if (!turnAckWaiter) return
    clearTimeout(turnAckWaiter.timer)
    turnAckWaiter.resolve(true)
    turnAckWaiter = null
  }

  function syncReplyBubble(text: string) {
    const pet = usePetStore()
    if (pet.isChatOpen || pet.isReminderBubbleActive()) return
    const trimmed = stripMoodTags(text.trim())
    if (!trimmed) return
    if (recording || talking.value) {
      if (pet.isVoiceBubbleActive()) pet.updateVoiceBubble(trimmed)
      else pet.showVoiceBubble(trimmed)
    }
  }

  function finishTextTurn() {
    clearTtsWatchdog()
    clearTurnAckWait()
    const pendingPartial = partialText.value.trim()
    if (pendingPartial) {
      commitHeardText(pendingPartial)
    }
    partialText.value = ''
    textSending = false
    pendingTextTurn = null
    textViaRest = false
    const finalReply = replyText.value.trim()
    const hadVoice = ttsPlayer.hadPlayback
    if (finalReply) syncReplyBubble(finalReply)
    replyText.value = ''
    if (recording) {
      // gate dismiss / ASR 空：无 TTS 可播，立即回 resting，避免 processing 阶段丢麦
      if (!finalReply && !hadVoice) {
        usePetStore().releaseVoiceBubble(HEARD_BUBBLE_GRACE_MS)
        resetTurnTiming()
        ttsPlayer.resetTurn()
        enterResting('没听清或不需要回应，请再说一次')
        usePetStore().syncAnimationFromState()
        return
      }
      ttsPlayer.markDone(() => {
        if (finalReply) usePetStore().updateVoiceBubble(finalReply)
        usePetStore().releaseVoiceBubble(HEARD_BUBBLE_GRACE_MS)
        resetTurnTiming()
        ttsPlayer.resetTurn()
        let hint: string | undefined
        if (!finalReply) {
          hint = '没听清或不需要回应，请再说一次'
        } else if (!hadVoice && finalReply) {
          hint = isOpusDecodeSupported()
            ? '文字已回复，语音播放失败，请检查音量'
            : '文字已回复（当前环境不支持 Opus，已请求 MP3）'
        }
        enterResting(hint)
        usePetStore().syncAnimationFromState()
      })
    } else {
      usePetStore().releaseVoiceBubble(HEARD_BUBBLE_GRACE_MS)
      resetTurnTiming()
      setPhase('idle')
      statusText.value = '输入消息或开始语音对话'
      usePetStore().syncAnimationFromState()
    }
  }

  function clearSpeechEndSubmitTimer() {
    if (speechEndSubmitTimer) {
      clearTimeout(speechEndSubmitTimer)
      speechEndSubmitTimer = null
    }
  }

  function clearSilenceWatch() {
    if (silenceTimer) {
      clearInterval(silenceTimer)
      silenceTimer = null
    }
    clearSpeechEndSubmitTimer()
  }

  function clearTtsWatchdog() {
    if (ttsWatchdog) {
      clearTimeout(ttsWatchdog)
      ttsWatchdog = null
    }
  }

  function setPhase(next: TurnPhase) {
    phase = next
    resting.value = next === 'resting'
    setProcessing(next === 'processing' || next === 'agent_speaking')
    speechVad?.setPlaybackMode(next === 'agent_speaking')
  }

  function refreshRuntimeParams() {
    params = defaultRuntimeParams()
  }

  function stopLocalListening() {
    localStt?.stop()
  }

  function startLocalListening() {
    if (effectiveSttMode !== 'local' || !recording) return
    if (!localStt) localStt = new LocalSTT()
    const rt = getRealtimeConfig()
    localStt.start(
      {
        onPartial: (text) => {
          if (phase === 'processing' || phase === 'agent_speaking') return
          partialText.value = text
          trackObjectIntentFromPartial(text)
          heardSpeech = true
          lastSpeechAt = Date.now()
          if (phase === 'resting') {
            return
          }
          if (phase === 'user_speaking') {
            statusText.value = '正在听...'
          }
        },
        onFinal: (text) => {
          handleLocalFinal(text)
        },
        onError: (msg) => {
          if (import.meta.env.DEV) console.warn('[localStt]', msg)
        },
      },
      rt.speechLocale,
    )
  }

  function handleLocalFinal(text: string) {
    if (!recording || phase === 'processing' || phase === 'agent_speaking') return
    if ([...text.trim()].length < params.minEndpointChars) return
    const now = Date.now()
    if (now - lastEndpointAt < params.endpointDebounceMs) return
    lastEndpointAt = now
    void submitLocalTranscript(text)
  }

  function submitLocalTranscript(text: string) {
    const trimmed = text.trim()
    if (!trimmed || submitLock) return
    if (!realtimeSession.isOpen()) {
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }

    stopLocalListening()
    clearSilenceWatch()
    submitLock = true
    setPhase('processing')
    heardSpeech = false
    partialText.value = ''
    statusText.value = '处理中...'
    turnStartAt = Date.now()
    playbackMarked = false

    commitUserMessage(trimmed, 'voice')
    replyText.value = ''
    startTtsWatchdog()

    void sendVisionFramesBeforeSubmit(trimmed).then(() => {
      const sent = realtimeSession.sendTextInput(trimmed, { voiceReply: true })
      if (!sent) {
        submitLock = false
        messages.value.pop()
        enterResting()
        statusText.value = '发送失败，请重试'
        return
      }
      setTimeout(() => {
        submitLock = false
      }, 800)
    })
  }

  function buildTurnEndSignals() {
    return {
      heardSpeech,
      lastSpeechAt,
      vadSpeaking: speechVad?.isSpeaking() ?? false,
      partialText: partialText.value,
      partialUpdatedAt,
      chunksSent: chunksSentCount,
      thinkingHoldUntil,
      silenceMsConfig: params.silenceMs,
      resumedAfterPauseProbe,
    }
  }

  /** 静音≥3s：眼抓拍 + 若像还在组织语言则延长等待 */
  async function runPauseThinkingProbe() {
    if (pauseProbeInFlight || phase !== 'user_speaking') return
    pauseProbeInFlight = true
    try {
      void maybeSendVisionFrame('pause_probe')
      void runFaceCheck()
      const decision = evaluateTurnEnd(buildTurnEndSignals())
      if (decision.extendHoldMs) {
        thinkingHoldUntil = Math.max(thinkingHoldUntil, Date.now() + decision.extendHoldMs)
        if (import.meta.env.DEV) {
          console.debug('[turn_end] pause_probe hold=%sms reason=%s', decision.extendHoldMs, decision.reason)
        }
      }
    } finally {
      pauseProbeInFlight = false
    }
  }

  function startSilenceWatch() {
    clearSilenceWatch()
    silenceTimer = setInterval(() => {
      if (!recording || phase !== 'user_speaking') return
      const signals = buildTurnEndSignals()
      const silence = Date.now() - signals.lastSpeechAt

      if (silence >= PAUSE_PROBE_MS && !pauseProbeDone && signals.heardSpeech) {
        pauseProbeDone = true
        void runPauseThinkingProbe()
      }

      const decision = evaluateTurnEnd(signals)
      // extendHold 仅在 runPauseThinkingProbe 里应用一次，避免每 200ms 滑动延长导致永不提交

      if (decision.ready) {
        void submitUtterance()
      }
      if (utteranceStartedAt > 0 && Date.now() - utteranceStartedAt >= MAX_UTTERANCE_MS) {
        void submitUtterance(true)
      }
    }, 200)
  }

  function setProcessing(v: boolean) {
    processingRef.value = v
  }

  /** Mochi goes back to sleep — mic stays open but nothing is uploaded. */
  function enterResting(hint?: string) {
    clearTtsWatchdog()
    stopStreamCheck()
    identityGate.resetTurn()
    setPhase('resting')
    submitLock = false
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    heardSpeech = false
    lastSpeechAt = 0
    utteranceStartedAt = 0
    bargeAccumMs = 0
    wakeAccumMs = 0
    ttsStartedAt = 0
    objectRefreshCountThisTurn = 0
    speechStartFrameSent = false
    lastFaceProbe = null
    ownerTurnUploadLock = false
    turnUploadStarted = false
    partialUpdatedAt = 0
    thinkingHoldUntil = 0
    pauseProbeDone = false
    pauseProbeInFlight = false
    resumedAfterPauseProbe = false
    resetTurnTiming()
    chunksSent.value = 0
    resetPcmRing()
    speechVad?.reset()
    ambientPresence.setOwnerSpeaking(false)
    if (recording) {
      statusText.value = hint ?? 'Mochi 在休息... 说话我就听'
      usePetStore().setAnimation('idle')
      if (effectiveSttMode === 'local') {
        startLocalListening()
      } else {
        startSilenceWatch()
      }
    } else {
      statusText.value = connected.value ? '点击开始对话' : ''
    }
  }

  /** Send recent ring-buffer audio so ASR receives speech that occurred before wake. */
  function flushPreRollAudio() {
    const vad = getRealtimeConfig().vad
    const vp = getVoiceprintConfig()
    const padSec = (vad.silero?.preSpeechPadMs ?? 300) / 1000
    const preRollSec = Math.min(vp.wakeProbeSec + padSec + 0.25, 2)
    const samples = snapshotRecentPcm(preRollSec)
    if (samples.length === 0) return

    const chunkSamples = 320 // 20 ms @ 16 kHz
    for (let i = 0; i < samples.length; i += chunkSamples) {
      const slice = samples.subarray(i, Math.min(i + chunkSamples, samples.length))
      const pcm = float32ToPcm16LE(slice)
      const boosted = amplifyPCM(pcm)
      const peak = pcmPeakLevel(boosted)
      uploadSeq++
      chunksSentCount++
      chunksSent.value = chunksSentCount
      if (peak > peakSeen) peakSeen = peak
      realtimeSession.sendAudio(arrayBufferToBase64(boosted), uploadSeq)
    }
  }

  /** Owner started speaking — wake up and begin uploading. */
  function wakeOnSpeech() {
    if (phase !== 'resting' || !recording) return
    usePetStore().clearEmotionHold()
    setPhase('user_speaking')
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    // 等检测到真实语音后再标记 heardSpeech，避免点击唤醒后静音误触发提交
    heardSpeech = false
    lastSpeechAt = 0
    utteranceStartedAt = Date.now()
    objectRefreshCountThisTurn = 0
    speechStartFrameSent = false
    // 保留唤醒时的人脸 probe，供 stream_check 立即融合（勿清空）
    chunksSent.value = 0
    partialText.value = ''
    partialUpdatedAt = 0
    pauseProbeDone = false
    pauseProbeInFlight = false
    resumedAfterPauseProbe = false
    thinkingHoldUntil = 0
    ambientPresence.setOwnerSpeaking(true)
    identityGate.markOwnerMatch()
    identityGate.resetTurn()
    ownerTurnUploadLock = true
    // 唤醒即连续上传（含点击 manual wake），避免 VAD 已触发但 PCM 未传导致 ASR 空
    turnUploadStarted = true
    realtimeSession.sendAudioStart()
    sendVisionFrameOnSpeechStart()
    flushPreRollAudio()
    startStreamCheck()
    startSilenceWatch()
    statusText.value = '正在听...'
  }

  /** Click pet while resting — start listening immediately. */
  async function wakeListening(opts?: WakeListeningOptions): Promise<WakeListeningResult> {
    if (phase !== 'resting' || !recording) {
      return { ok: false, reason: 'not_ready' }
    }
    if (!voiceprintReady()) {
      statusText.value = '请先在设置中录入主人声纹'
      return { ok: false, reason: 'voiceprint_missing' }
    }
    if (effectiveSttMode === 'local') {
      setPhase('user_speaking')
      statusText.value = '正在听...'
      ambientPresence.setOwnerSpeaking(true)
      startLocalListening()
      sendVisionFrameOnSpeechStart()
      return { ok: true }
    }
    if (!realtimeSession.isOpen()) {
      await connectIfOwner()
      if (!realtimeSession.isOpen()) {
        return { ok: false, reason: 'disconnected' }
      }
    }
    // 用户主动点击：明确意图，直接开始上传，不再做唤醒前声纹探测（避免「说了没反应」）
    if (opts?.manual) {
      identityGate.markOwnerMatch()
      wakeOnSpeech()
      return { ok: true }
    }
    const probe = await probeWakeFromResting()
    if (probe === 'ok') return { ok: true }
    if (probe === 'not_owner') {
      // 唤醒前补一次人脸（lastFaceProbe 可能尚未有 speech_start 帧）
      if (!faceSupportsOwner(lastFaceProbe)) {
        const face = await quickFaceProbeForWake()
        if (faceSupportsOwner(face)) {
          identityGate.markOwnerMatch()
          wakeOnSpeech()
          return { ok: true }
        }
      } else {
        identityGate.markOwnerMatch()
        wakeOnSpeech()
        return { ok: true }
      }
      // 仅高置信非主人才 TTS 拒答；否则静默
      if (lastWakeProbeScore != null && lastWakeProbeScore < NON_OWNER_REPLY_SCORE_MAX) {
        void sendNonOwnerReply(true, lastWakeProbeScore)
      }
      return { ok: false, reason: 'not_owner' }
    }
    if (probe === 'not_speech') return { ok: false, reason: 'not_speech' }
    if (phase !== 'resting') return { ok: true }
    return { ok: false, reason: 'not_ready' }
  }

  function handleAsrEndpoint(text: string) {
    if (!params.endpointingEnabled) return
    if (!heardSpeech || phase !== 'user_speaking') return
    if ([...text.trim()].length < params.minEndpointChars) return
    const now = Date.now()
    if (now - lastEndpointAt < params.endpointDebounceMs) return
    lastEndpointAt = now
    void submitUtterance()
  }

  function submitUtterance(force = false) {
    void submitUtteranceAsync(force)
  }

  async function maybeSendVisionFrame(reason: VisionFrameReason = 'audio_end') {
    const cfg = getClientConfig()
    if (!cfg.visionEnabled) {
      console.warn('[vision] skip_send reason=server_disabled trigger=%s', reason)
      return
    }
    if (!isVisionCaptureEnabled()) {
      console.warn('[vision] skip_send reason=client_opt_out trigger=%s', reason)
      return
    }
    if (reason === 'speech_start' && !cfg.visionSnapshotOnSpeechStart) {
      return
    }
    if (reason === 'audio_end' && cfg.visionSnapshotOnAudioEnd === false) {
      return
    }
    if (reason === 'object_refresh' && cfg.visionSnapshotOnObjectIntent === false) {
      return
    }

    const start = Date.now()
    const buf = visionSession.isActive()
      ? await visionSession.grabSnapshot()
      : await captureOwnerFaceJPEG()
    if (!buf) {
      console.warn(
        '[vision] skip_send reason=capture_failed trigger=%s elapsed_ms=%d',
        reason,
        Date.now() - start,
      )
      return
    }
    const b64 = jpegToBase64(buf)
    let faceProbe: { match: boolean; score: number; detected: boolean } | undefined
    const fp = getFaceprintConfig()
    const shouldProbeFace =
      fp.enabled &&
      faceprintReady() &&
      (reason !== 'speech_start' || fp.probeOnSpeechStart)
    if (shouldProbeFace) {
      const face = await probeFaceFromJPEG(buf)
      if (face) {
        faceProbe = face
        if (import.meta.env.DEV) {
          console.debug('[faceprint] frame_probe reason=%s match=%s score=%s', reason, face.match, face.score.toFixed(3))
        }
      }
    }
    const ok = realtimeSession.sendVisionFrame(b64, { reason, faceProbe })
    console.info(
      '[vision] frame_sent ok=%s trigger=%s jpeg_bytes=%d b64_len=%d elapsed_ms=%d',
      ok,
      reason,
      buf.byteLength,
      b64.length,
      Date.now() - start,
    )
  }

  /** speech_start 预拍帧（每 turn 一次，不阻塞 VAD）。 */
  function sendVisionFrameOnSpeechStart() {
    if (speechStartFrameSent) return
    speechStartFrameSent = true
    void maybeSendVisionFrame('speech_start')
  }

  /** P2：ASR partial 命中举物语义时 mid-turn 补拍（延迟一拍，等用户举物对准镜头）。 */
  function trackObjectIntentFromPartial(text: string) {
    if (!text.trim() || objectRefreshCountThisTurn > 0) return
    if (!getClientConfig().visionSnapshotOnObjectIntent) return
    if (!looksLikeObjectQuery(text)) return
    objectRefreshCountThisTurn++
    setTimeout(() => {
      void maybeSendVisionFrame('object_refresh')
    }, OBJECT_FRAME_HOLD_MS)
  }

  /** P2：submit 前若像举物问句，留时间举物后再抓拍，最后 audio_end 帧覆盖。 */
  async function sendVisionFramesBeforeSubmit(turnText: string) {
    if (
      getClientConfig().visionSnapshotOnObjectIntent &&
      looksLikeObjectQuery(turnText)
    ) {
      await new Promise((r) => setTimeout(r, OBJECT_FRAME_HOLD_MS))
      await maybeSendVisionFrame('object_refresh')
      await new Promise((r) => setTimeout(r, 120))
    }
    await maybeSendVisionFrame('audio_end')
  }

  async function submitUtteranceAsync(force = false) {
    if (!talking.value && !recording) {
      statusText.value = '请先点击开始对话'
      return
    }
    if (phase === 'resting') return
    if (phase !== 'user_speaking') {
      if (phase === 'processing') statusText.value = '处理中，请稍候...'
      else if (phase === 'agent_speaking') statusText.value = 'Mochi 正在说话，请稍候或大声说话打断'
      return
    }
    if (submitLock && !force) return

    if (!force) {
      if (speechVad?.isSpeaking()) return
      if (!evaluateTurnEnd(buildTurnEndSignals()).ready) return
    }

    if (chunksSentCount === 0) {
      if (partialText.value.trim()) {
        commitHeardText(partialText.value, { dismissed: true, dismissReason: '未检测到有效语音' })
      }
      statusText.value = '没听到声音，请再说一次'
      usePetStore().showSpeechBubble('没听到声音，请大声一点~', 3000)
      enterResting()
      return
    }
    if (!force && peakSeen < 0.03) {
      if (partialText.value.trim()) {
        commitHeardText(partialText.value, { dismissed: true, dismissReason: '音量过低' })
      }
      enterResting()
      return
    }

    if (!realtimeSession.isOpen()) {
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }

    clearSilenceWatch()
    stopStreamCheck()
    submitLock = true
    setPhase('processing')
    heardSpeech = false
    lastSpeechAt = 0
    utteranceStartedAt = 0
    bargeAccumMs = 0
    speechVad?.reset()
    statusText.value = '处理中...'
    turnStartAt = Date.now()
    playbackMarked = false
    ttsPlayer.resetTurn()

    await sendVisionFramesBeforeSubmit(partialText.value.trim())

    const sent = realtimeSession.sendAudioEnd()
    if (!sent) {
      submitLock = false
      enterResting()
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }

    setTimeout(() => {
      submitLock = false
    }, 800)
  }

  function handleVadEvent(ev: VADEvent) {
    if (!recording) return

    if (phase === 'resting' && ev === 'speech_start') {
      void tryWakeFromResting()
      return
    }

    if (phase !== 'user_speaking') return

    if (ev === 'speech_start') {
      clearSpeechEndSubmitTimer()
      heardSpeech = true
      lastSpeechAt = Date.now()
      // 句中 3s 停顿后续说：解除 thinking hold，句末静音走正常仲裁
      if (pauseProbeDone) {
        resumedAfterPauseProbe = true
        thinkingHoldUntil = 0
      }
      statusText.value = '正在听...'
      sendVisionFrameOnSpeechStart()
      return
    }

    if (ev === 'speech_end' && heardSpeech) {
      // 提交时机由 silence watch + turnEndArbiter 统一仲裁（含 3s pause_probe）
      clearSpeechEndSubmitTimer()
    }
  }

  function bargeIn() {
    if (phase !== 'agent_speaking') return
    if (!ttsPlayer.hadPlayback && !replyText.value.trim()) return
    ttsPlayer.stop()
    clearTtsWatchdog()
    replyText.value = ''
    usePetStore().releaseVoiceBubble(0)
    realtimeSession.sendInterrupt()
    enterResting()
  }

  function interruptForReminder() {
    ttsPlayer.stop()
    clearTtsWatchdog()
    if (phase === 'agent_speaking') {
      replyText.value = ''
      usePetStore().releaseVoiceBubble(0)
      realtimeSession.sendInterrupt()
      enterResting()
    }
  }

  function checkBargeIn(peak: number) {
    if (phase !== 'agent_speaking') {
      bargeAccumMs = 0
      return
    }
    if (Date.now() - ttsStartedAt < params.echoGuardMs) return

    if (peak >= params.bargeInPeak) {
      bargeAccumMs += 20
      if (bargeAccumMs >= params.bargeInMs) {
        bargeIn()
      }
    } else {
      bargeAccumMs = 0
    }
  }

  function startTtsWatchdog() {
    clearTtsWatchdog()
    ttsWatchdog = setTimeout(() => {
      if (phase !== 'agent_speaking' && phase !== 'processing') return
      ttsPlayer.stop()
      if (recording) {
        enterResting()
        statusText.value = '语音超时，请继续说话'
        return
      }
      textSending = false
      setPhase('idle')
      statusText.value = '回复超时，请再发一次'
    }, TTS_WATCHDOG_MS)
  }

  async function initVad() {
    speechVad?.destroy()
    speechVad = new HybridSpeechVad(handleVadEvent, getRealtimeConfig().vad)
    await speechVad.init()
  }

  async function pauseVoiceForText() {
    if (!recording) return
    clearSilenceWatch()
    clearTtsWatchdog()
    ttsPlayer.stop()
    submitLock = false
    micLevel.value = 0
    heardSpeech = false
    resting.value = false
    partialText.value = ''
    speechVad?.destroy()
    speechVad = null
    stopLocalListening()
    localStt = null
    recording = false
    talking.value = false
    await capture.stop()
    await stopVisionSession()
    setPhase('idle')
  }

  async function sendTextViaRest(trimmed: string) {
    if (textViaRest) return
    textViaRest = true
    clearTurnAckWait()
    statusText.value = 'Mochi 正在想...'
    replyText.value = ''
    try {
      const reply = await streamChatMessage(trimmed, (token) => {
        replyText.value += token
        syncReplyBubble(replyText.value)
      })
      if (reply) {
        commitAssistantMessage(reply)
      }
      finishTextTurn()
    } catch (e) {
      textViaRest = false
      textSending = false
      pendingTextTurn = null
      clearTtsWatchdog()
      setPhase('idle')
      const last = messages.value[messages.value.length - 1]
      if (last?.role === 'user' && last.content === trimmed) {
        messages.value.pop()
      }
      statusText.value = e instanceof Error ? e.message : '发送失败，请重试'
    }
  }

  async function sendTextMessage(text: string) {
    const trimmed = text.trim()
    if (!trimmed) return
    if (textSending) {
      statusText.value = 'Mochi 正在回复，请稍候...'
      return
    }

    await pauseVoiceForText()

    if (processingRef.value && !recording) {
      statusText.value = 'Mochi 正在回复，请稍候...'
      return
    }

    pendingTextTurn = trimmed
    textSending = true
    commitUserMessage(trimmed, 'text')
    partialText.value = ''
    replyText.value = ''
    setPhase('processing')
    startTtsWatchdog()
    statusText.value = 'Mochi 正在想...'
    turnStartAt = Date.now()
    playbackMarked = false

    await connect()
    if (!realtimeSession.isOpen()) {
      await sendTextViaRest(trimmed)
      return
    }

    const ackWait = beginTurnAckWait(TEXT_TURN_ACK_MS)
    const sent = realtimeSession.sendTextInput(trimmed)
    if (!sent) {
      await sendTextViaRest(trimmed)
      return
    }

    const acked = await ackWait
    if (!acked && textSending && pendingTextTurn === trimmed) {
      await sendTextViaRest(trimmed)
    }
  }

  async function connectInternal(): Promise<void> {
    if (!(await shouldOwnVoice())) {
      const owner = getStoredVoiceOwner()
      if (voiceWindow === 'pet' && owner === 'chat') {
        statusText.value = '聊天窗口占用语音连接，请先关闭聊天'
      } else if (voiceWindow === 'chat' && owner !== 'chat') {
        statusText.value = '语音通道未就绪，请关闭聊天后重新打开'
      } else {
        statusText.value = '连接失败，请稍后再试'
      }
      return
    }
    if (connected.value && realtimeSession.isOpen()) return

    if (connected.value && !realtimeSession.isOpen()) {
      connected.value = false
      detachHandler()
      intentionalDisconnect = true
      realtimeSession.disconnect()
      intentionalDisconnect = false
    }

    statusText.value = '连接中...'

    detachHandler()
    unsub = realtimeSession.on(handleEvent)
    try {
      await realtimeSession.connect()
      await realtimeSession.sendClientCaps()
    } catch {
      connected.value = false
      detachHandler()
      statusText.value = '连接失败，请关闭面板重新打开'
      return
    }
    connected.value = true
    realtimeSession.sendPrewarm()
    statusText.value = recording ? 'Mochi 在休息... 说话我就听' : '输入消息或开始语音对话'
  }

  async function connect(): Promise<void> {
    if (connectFlight) return connectFlight
    connectFlight = connectInternal().finally(() => {
      connectFlight = null
    })
    return connectFlight
  }

  async function reconnectAfterDisconnect(restoreVoice: boolean) {
    if (reconnecting) return
    reconnecting = true
    statusText.value = '连接断开，正在重连...'
    try {
      await connectIfOwner()
      if (restoreVoice && connected.value && capture.isActive) {
        talking.value = true
        recording = true
        enterResting()
      } else if (!restoreVoice) {
        talking.value = false
        recording = false
        setPhase('idle')
        resting.value = false
      }
    } catch {
      talking.value = false
      recording = false
      setPhase('idle')
      resting.value = false
      statusText.value = '连接断开，请关闭面板重新打开'
    } finally {
      reconnecting = false
    }
  }

  /** Keep /ws/voice open for push reminders (pet window only, chat closed). */
  async function ensurePushConnected() {
    if (!(await shouldOwnVoice())) return
    if (connected.value && realtimeSession.isOpen()) return
    try {
      await connect()
    } catch (e) {
      console.warn('[realtime] push connect skipped', e)
    }
  }

  function disconnect() {
    intentionalDisconnect = true
    void endConversation()
    detachHandler()
    realtimeSession.disconnect()
    connected.value = false
    statusText.value = ''
    intentionalDisconnect = false
  }

  async function startCloudTalk() {
    await initVad()

    try {
      await capture.start((pcm, _seq) => {
        const rawFloat = pcmToFloat(pcm)
        if (phase === 'resting' || phase === 'user_speaking') {
          pushPcmRing(rawFloat)
        }

        const boosted = amplifyPCM(pcm)
        const peak = pcmPeakLevel(boosted)
        micLevel.value = peak

        if (phase === 'resting') {
          speechVad?.feed(pcmToFloat(boosted))
          const vadSpeaking = speechVad?.isSpeaking() ?? false
          // 需 VAD 认为在说话，或能量明显偏高，才尝试唤醒（避免杂音误触）
          if (peak >= params.wakePeak && (vadSpeaking || peak >= params.wakePeak * 2)) {
            wakeAccumMs += 20
            if (wakeAccumMs >= WAKE_CONFIRM_MS) {
              wakeAccumMs = 0
              void tryWakeFromResting()
            }
          } else {
            wakeAccumMs = 0
          }
          return
        }

        if (phase === 'user_speaking') {
          const peakOk = peak >= params.tailSpeechPeak || (speechVad && speechVad.isSpeaking())
          const hasSpeechEnergy = peakOk || (speechVad?.isSpeaking() ?? false)
          if (hasSpeechEnergy) {
            if (peakOk && (ownerTurnUploadLock || identityGate.shouldAllowUpload())) {
              heardSpeech = true
              lastSpeechAt = Date.now()
              // VAD 未切 speech_start 时，能量续说同样解除 pause hold
              if (pauseProbeDone && !resumedAfterPauseProbe) {
                resumedAfterPauseProbe = true
                thinkingHoldUntil = 0
              }
            }
            turnUploadStarted = true
          }
          speechVad?.feed(pcmToFloat(boosted))
          // 首包语音前不传；首包后连续上传（含句中停顿），保证流式 ASR 不断流
          if (!turnUploadStarted) {
            return
          }
          if (!ownerTurnUploadLock && !identityGate.shouldAllowUpload()) {
            return
          }
          uploadSeq++
          chunksSentCount++
          chunksSent.value = chunksSentCount
          if (peak > peakSeen) peakSeen = peak
          realtimeSession.sendAudio(arrayBufferToBase64(boosted), uploadSeq)
          return
        }

        if (phase === 'agent_speaking') {
          speechVad?.feed(pcmToFloat(boosted))
          checkBargeIn(peak)
        }
      })
    } catch (e) {
      const err = e as DOMException
      if (err?.name === 'NotAllowedError') {
        statusText.value = micPermissionDeniedMessage()
      } else if (err?.name === 'NotFoundError') {
        statusText.value = '未检测到麦克风设备'
      } else {
        statusText.value = '无法启动麦克风'
      }
      talking.value = false
      recording = false
      setPhase('idle')
      resting.value = false
      return
    }

    recording = true
    talking.value = true
    enterResting()
  }

  async function startLocalTalk() {
    localStt = new LocalSTT()
    try {
      recording = true
      talking.value = true
      enterResting()
      startLocalListening()
    } catch {
      stopLocalListening()
      localStt = null
      recording = false
      talking.value = false
      effectiveSttMode = 'cloud'
      statusText.value = '本地语音识别不可用，切换云端模式...'
      await startCloudTalk()
    }
  }

  async function startTalk(): Promise<boolean> {
    if (recording) return true

    await initClientConfig().catch(() => {})
    refreshRuntimeParams()

    await connect()
    if (!realtimeSession.isOpen()) {
      const owner = getStoredVoiceOwner()
      if (voiceWindow === 'chat' && owner !== 'chat') {
        statusText.value = '语音通道未就绪，请关闭聊天后重新打开'
      } else {
        statusText.value = '连接失败，请稍后再试'
      }
      return false
    }

    realtimeSession.sendPrewarm()

    effectiveSttMode = resolveSttMode(getRealtimeConfig(), isLocalSttSupported())

    await speakerVerifier.init().catch(() => {})
    await faceVerifier.init().catch(() => {})
    await soundClassifier.init().catch(() => {})
    await loadOwnerVoiceprint()
    await loadOwnerFaceprint()

    if (!voiceprintReady()) {
      statusText.value = '请先在设置中录入主人声纹（设置 → 主人声纹）'
      return false
    }

    await pauseAmbientMicForTalk()

    partialText.value = ''
    replyText.value = ''
    ttsPlayer.stop()
    clearTtsWatchdog()
    submitLock = false
    micLevel.value = 0

    if (effectiveSttMode === 'local') {
      await startLocalTalk()
      await startVisionSession().catch(() => {})
      return recording
    }

    await startCloudTalk()
    await startVisionSession().catch(() => {})
    // 保持 resting，等用户说话（VAD）或再次点击（manual wake）；勿在此自动 wakeOnSpeech
    return recording
  }

  async function initAmbientPresence() {
    await initClientConfig().catch(() => {})
    await speakerVerifier.init().catch(() => {})
    await loadOwnerVoiceprint()
    await startAmbientPresenceService(ownerEmbedding)
  }

  async function endConversation() {
    if (!recording && !talking.value) return
    clearSilenceWatch()
    clearTtsWatchdog()
    recording = false
    talking.value = false
    submitLock = false
    micLevel.value = 0
    heardSpeech = false
    resting.value = false
    ttsPlayer.stop()
    speechVad?.destroy()
    speechVad = null
    stopLocalListening()
    localStt = null
    effectiveSttMode = 'cloud'
    setPhase('idle')
    ambientPresence.setOwnerSpeaking(false)
    if (capture.isActive) {
      await capture.stop()
    }
    await stopVisionSession()
    statusText.value = connected.value ? '点击开始对话' : ''
    await resumeAmbientMicAfterTalk(ownerEmbedding)
  }

  function handleEvent(ev: RealtimeEvent) {
    const pet = usePetStore()

    switch (ev.type) {
      case 'session_start':
        sessionId.value = ev.sessionId
        break
      case 'barge_in_config':
        params.echoGuardMs = ev.echoGuardMs
        params.bargeInPeak = ev.peakThreshold
        params.bargeInMs = ev.bargeInMs
        console.info(
          `[realtime] barge_in_config aec=${ev.aecEnabled} echo_guard_ms=${ev.echoGuardMs}`,
        )
        break
      case 'asr_partial':
        partialText.value = ev.text
        partialUpdatedAt = Date.now()
        trackObjectIntentFromPartial(ev.text)
        if (ev.text.trim() && !pet.isChatOpen) {
          pet.showPersistentBubble(`"${stripMoodTags(ev.text.trim())}"`)
        }
        if (ev.sentenceEnd) {
          handleAsrEndpoint(ev.text)
        }
        break
      case 'asr_final': {
        if (textSending) {
          partialText.value = ''
          break
        }
        const finalText = (ev.text.trim() || partialText.value.trim())
        partialText.value = ''
        if (!finalText) {
          startTtsWatchdog()
          break
        }
        commitUserMessage(finalText, 'voice')
        if (!pet.isChatOpen) {
          pet.showVoiceBubble(`“${finalText}”`)
        }
        replyText.value = ''
        setPhase('processing')
        startTtsWatchdog()
        statusText.value = 'Mochi 正在想...'
        break
      }
      case 'llm_token':
        if (textViaRest) break
        if (recording && phase === 'processing') {
          statusText.value = 'Mochi 正在回复...'
        }
        replyText.value += ev.token
        syncReplyBubble(replyText.value)
        break
      case 'llm_done':
        if (textViaRest) break
        replyText.value = ev.text
        syncReplyBubble(ev.text)
        commitAssistantMessage(ev.text)
        if (recording) {
          if (phase !== 'agent_speaking') {
            statusText.value = 'Mochi 正在回复...'
          }
        } else if (textSending) {
          finishTextTurn()
        } else {
          statusText.value = 'Mochi 已回复'
        }
        break
      case 'tts_audio': {
        const audio =
          ev.audioBuffer && ev.audioBuffer.byteLength > 0 ? ev.audioBuffer : ev.pcm
        if (!audio || (typeof audio !== 'string' && audio.byteLength === 0)) break
        setPhase('agent_speaking')
        if (!ttsStartedAt) ttsStartedAt = Date.now()
        statusText.value = 'Mochi 正在说话...（大声说话可打断）'
        ttsPlayer.enqueue(audio, ev.format, markPlaybackStart, ev.seq)
        break
      }
      case 'tts_segment_done':
        ttsPlayer.flushSegment()
        break
      case 'tts_done':
        if (phase !== 'resting' && phase !== 'idle') {
          finishTextTurn()
        } else {
          clearTtsWatchdog()
          textSending = false
          resetTurnTiming()
        }
        break
      case 'turn_metrics':
        lastTurnMetrics = ev.metrics
        if (import.meta.env.DEV) {
          console.debug('[realtime] turn_metrics', ev.metrics)
          if (ev.metrics.visionMs >= 0) {
            console.info('[realtime] vision_ms=%d', ev.metrics.visionMs)
          }
          if (ev.metrics.perceiveParallelMs >= 0) {
            console.info('[realtime] perceive_parallel_ms=%d', ev.metrics.perceiveParallelMs)
          }
        }
        break
      case 'turn_ack':
        signalTurnAck()
        if (turnStartAt <= 0) turnStartAt = Date.now()
        setPhase('processing')
        startTtsWatchdog()
        statusText.value = 'Mochi 正在想...'
        break
      case 'interrupted':
        ttsPlayer.stop()
        clearTtsWatchdog()
        textSending = false
        pet.releaseVoiceBubble(0)
        enterResting()
        pet.setAnimation('happy')
        break
      case 'animation':
        // 会话阶段动画不覆盖 emotion_state 驱动的表情（state_update）
        if (ev.state === 'listening') pet.setAnimation('happy')
        else if (ev.state === 'idle') pet.syncAnimationFromState()
        break
      case 'error':
        ttsPlayer.stop()
        clearTtsWatchdog()
        textSending = false
        replyText.value = ''
        if (ev.code === 'TTS_FAILED') {
          const hasAssistant = messages.value.some((m) => m.role === 'assistant')
          if (recording) {
            enterResting()
            statusText.value = hasAssistant
              ? '语音播放失败了，回复已在上面~'
              : '语音合成暂时不可用，请稍后再试'
          } else {
            setPhase('idle')
            statusText.value = hasAssistant ? 'Mochi 已回复（语音不可用）' : '语音合成暂时不可用'
          }
          break
        }
        if (ev.code === 'LLM_EMPTY') {
          if (recording) {
            enterResting()
          } else {
            setPhase('idle')
          }
          break
        }
        if (ev.code !== 'ASR_FAILED') {
          commitAssistantMessage(ev.message)
        }
        if (recording) {
          enterResting()
          statusText.value = ev.message + '（可以继续说）'
        } else {
          setPhase('idle')
          statusText.value = ev.message
        }
        break
      case 'proactive_message':
        interruptForReminder()
        handleProactiveMessage(
          { message: ev.message, animation: ev.animation },
          { priority: true },
        )
        commitAssistantMessage(ev.message)
        break
      case 'disconnected':
        connected.value = false
        const restoreVoice = recording && talking.value && !intentionalDisconnect

        clearSilenceWatch()
        clearTurnAckWait()

        if (textSending && pendingTextTurn) {
          clearTtsWatchdog()
          detachHandler()
          void sendTextViaRest(pendingTextTurn)
          statusText.value = '连接断开，改用文字通道...'
          void connectIfOwner().catch(() => {})
          break
        }

        clearTtsWatchdog()
        textSending = false
        pendingTextTurn = null
        detachHandler()

        if (intentionalDisconnect) {
          intentionalDisconnect = false
          if (!restoreVoice) {
            talking.value = false
            recording = false
            setPhase('idle')
            resting.value = false
          }
          break
        }

        if (!restoreVoice) {
          talking.value = false
          recording = false
          setPhase('idle')
          resting.value = false
        }

        void reconnectAfterDisconnect(restoreVoice)
        break
    }
  }

  return {
    connected,
    talking,
    resting,
    statusText,
    partialText,
    replyText,
    messages,
    sessionId,
    micLevel,
    chunksSent,
    processing: processingRef,
    userSpeaking,
    connect,
    connectIfOwner,
    setVoiceWindow,
    yieldVoiceConnection,
    ensurePushConnected,
    disconnect,
    startTalk,
    initAmbientPresence,
    wakeListening,
    sendTextMessage,
    loadHistory,
    appendAssistantMessage: commitAssistantMessage,
    interruptForReminder,
    submitUtterance,
    endConversation,
    stopTalk: submitUtterance,
    lastTurnMetrics,
  }
})
