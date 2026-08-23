import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  PCMCapture,
  arrayBufferToBase64,
  pcmPeakLevel,
  amplifyPCM,
  float32ToPcm16LE,
} from '@/services/pcmCapture'
import { realtimeSession, type RealtimeEvent, type TurnMetrics } from '@/services/realtimeSession'
import { usePetStore } from '@/stores/petStore'
import { TTSAudioQueue, isOpusDecodeSupported } from '@/services/ttsAudioPlayer'
import { HybridSpeechVad, pcmToFloat, type VADEvent } from '@/services/sileroSpeechVad'
import { isTauri } from '@/services/chatWindow'
import { SpeakerVerifier } from '@/services/speakerVerifier'
import { SoundEventClassifier } from '@/services/soundEventClassifier'
import { getRealtimeConfig, getVoiceprintConfig, getFaceprintConfig, getPresenceConfig, getClientConfig, initClientConfig } from '@/config'
import { micPermissionDeniedMessage } from '@/utils/micPermission'
import { stripMoodTags } from '@/utils/stripMoodTags'
import {
  captureOwnerFaceJPEG,
  isVisionCaptureEnabled,
  prewarmVisionSession,
  stopVisionSession,
  visionSession,
} from '@/services/visionCapture'
import {
  bootstrapAmbientPresence as startAmbientPresenceService,
  pauseAmbientMicForTalk,
  resumeAmbientMicAfterTalk,
  ambientPresence,
} from '@/services/ambientMic'
import { handleProactiveMessage, wasProactiveRecentlyShown } from '@/services/proactiveHandler'
import { getWakeGreetingText } from '@/services/wakeGreeting'
import {
  streamChatMessage,
  getVoiceprintStatus,
  getFaceprintStatus,
  getUserPreferences,
} from '@/services/api'
import {
  cacheVoiceprintEmbedding,
  readCachedVoiceprintEmbedding,
  clearVoiceprintEmbeddingCache,
} from '@/services/voiceprintCache'
import {
  type VoiceOwner,
  claimVoiceOwner,
  getStoredVoiceOwner,
  isChatWindowVisible,
} from '@/services/voiceSessionOwner'
import { identityGate } from '@/services/identityGate'
import {
  createPerceptionOrchestrator,
  PAUSE_PROBE_MS,
  shouldStartPeriodicFaceCheck,
  shouldStartStreamCheck,
  type PerceptionPhase,
} from '@/services/perception'
import {
  getPeriodicFaceCheckIntervalMs,
  getStreamCheckGraceMs,
  getStreamCheckIntervalMs,
} from '@/services/perception/earChannel'
import { FACE_RECOGNITION_ENABLED } from '@/services/modelPaths'
import { FaceVerifier, FACE_OWNER_BOOST_SCORE } from '@/services/faceVerifier'
import {
  evaluateTurnEnd,
  isComposingExpression,
  THINKING_HOLD_EXTEND_MS,
} from '@/services/turnEndArbiter'
import { startEventLoopProbe, onEventLoopLag } from '@/services/eventLoopProbe'
import { recordTurnMetricsBaseline } from '@/services/turnMetricsBaseline'
import { textContainsPetName } from '@/services/petNameWake'

/**
 * Turn phases — like talking to a person:
 * resting: mic monitors locally, Mochi sleeps, no upload
 * user_speaking: owner voice detected, recording & uploading
 * processing / agent_speaking: Mochi thinks & replies
 */
export type TurnPhase = 'idle' | 'resting' | 'user_speaking' | 'processing' | 'agent_speaking'

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
const MAX_UTTERANCE_MS = 8000
/** 空识别提示最短间隔，避免连续弹「没听到声音」。 */
const EMPTY_PROMPT_COOLDOWN_MS = 12000
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
  /** DEV：当前本地 STT 后端（xasr / webspeech / cloud）。 */
  const sttBackendLabel = ref<'xasr' | 'webspeech' | 'cloud' | ''>('')
  /** DEV：当前 TTS 后端（matcha / cloud）。 */
  const ttsBackendLabel = ref<'matcha' | 'cloud' | ''>('')
  /** DEV：sidecar 是否在线（Phase4 固定 false，保留供诊断面板）。 */
  const xasrSidecarReachable = ref<boolean | null>(null)
  /** DEV：X-TTS sidecar 是否在线（Phase4 固定 false）。 */
  const xttsSidecarReachable = ref<boolean | null>(null)
  let lastEmptyPromptAt = 0
  const replyText = ref('')
  const messages = ref<ChatMessage[]>([])
  const sessionId = ref('')
  const micLevel = ref(0)
  const chunksSent = ref(0)
  const processingRef = ref(false)
  /** DEV 诊断：当前轮次相位与最近一次检测到语音的时间戳。 */
  const turnPhase = ref<TurnPhase>('idle')
  /** DEV 诊断：Orchestrator 内部感知相位（listen/endpoint/think/speak）。 */
  const perceptionPhase = ref<PerceptionPhase>('idle')
  /** DEV 诊断：服务端 SessionState（listening/thinking/speaking/idle）。 */
  const serverSessionState = ref<string>('idle')
  const lastSpeechAtMs = ref(0)
  const eventLoopLagMs = ref(0)

  const userSpeaking = computed(
    () => talking.value && !resting.value && !processingRef.value,
  )

  const capture = new PCMCapture()
  const ttsPlayer = new TTSAudioQueue()
  /** Phase4：语音链路固定云端 STT。 */
  const effectiveSttMode = 'cloud' as const
  let params = defaultRuntimeParams()
  let recording = false
  let phase: TurnPhase = 'idle'
  let uploadSeq = 0
  let chunksSentCount = 0
  let peakSeen = 0
  let heardSpeech = false
  let lastSpeechAt = 0
  let utteranceStartedAt = 0
  /** VAD speech_end 时刻，供 cloud turn-end 快速提交 */
  let speechEndedAt = 0
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
  /** 语音轮次已提交、等待 LLM/TTS 的文本（WS 断线 REST 回退用）。 */
  let pendingVoiceTurn: string | null = null
  let textViaRest = false
  let voiceViaRest = false
  let turnAckWaiter: { resolve: (ok: boolean) => void; timer: ReturnType<typeof setTimeout> } | null =
    null
  let turnStartAt = 0
  let playbackMarked = false
  let lastEndpointAt = 0
  let lastTurnMetrics = ref<TurnMetrics | null>(null)
  /** Which window may hold /ws/voice: pet | chat | inline (browser single-window). */
  let voiceWindow: VoiceOwner | 'inline' = 'inline'
  let connectFlight: Promise<void> | null = null
  /** 递增以作废进行中的 connect()，避免后端重启后 connectFlight 永久卡住。 */
  let connectGeneration = 0

  function resetConnectFlight(reason?: string) {
    connectGeneration += 1
    connectFlight = null
    if (reason) {
      console.warn('[realtime] reset connect flight:', reason)
    }
  }
  let intentionalDisconnect = false
  let reconnecting = false
  let eventLoopProbeStarted = false
  /** 在场闲聊：speak_only 播完后进入连续聆听。 */
  let presenceChatListenAfterTts = false

  /** 与提醒 TTS 共用开关；关闭时仅气泡、不发起 speak_only。 */
  function cloudVoiceEnabled(): boolean {
    if (typeof localStorage === 'undefined') return true
    return localStorage.getItem('mochi_reminder_voice') !== '0'
  }

  /** 云端短句播报前停掉在途 TTS，避免叠音。 */
  function stopActiveTtsForCloudSpeak() {
    ttsPlayer.stop()
    if (phase === 'agent_speaking' || phase === 'processing') {
      realtimeSession.sendInterrupt()
    }
  }

  /** 提交前停播残留 TTS（连续聆听时上一句可能仍在队列）。 */
  function stopPlaybackBeforeSubmit() {
    ttsPlayer.stop()
    if (phase === 'agent_speaking') {
      realtimeSession.sendInterrupt()
    }
  }

  function resetTurnAbort() {
    turnAbortController?.abort()
    turnAbortController = new AbortController()
  }

  function cancelTurnAbort() {
    turnAbortController?.abort()
    turnAbortController = null
  }

  function ensureEventLoopProbe() {
    if (eventLoopProbeStarted || !import.meta.env.DEV) return
    eventLoopProbeStarted = true
    startEventLoopProbe(getClientConfig().eventLoopProbeMs || 1000)
    onEventLoopLag((lag) => {
      eventLoopLagMs.value = lag
    })
  }

  function touchLastSpeechAt(at = Date.now()) {
    lastSpeechAt = at
    lastSpeechAtMs.value = at
  }

  /** 提交前验主人声纹（客户端 gate 补验）。 */
  async function verifyOwnerBeforeSubmit(): Promise<boolean> {
    const vp = getVoiceprintConfig()
    if (!vp.required) return true
    if (ownerTurnUploadLock) return true
    const pcm = snapshotRecentPcm(Math.min(vp.verifyWindowSec, 3))
    const result = await verifyOwnerVoice(pcm, true)
    const fp = getFaceprintConfig()
    const face = lastFaceProbe ?? identityGate.lastFaceResult
    const mode = identityGate.applyIdentityResult(
      result ?? { match: false, score: 0 },
      face,
      vp,
      fp,
    )
    return mode === 'owner' || mode === 'owner_boost' || mode === 'open'
  }

  /** 本句是否确有有效上行音频（非纯 VAD 误触 / 门控未开）。 */
  function hadMeaningfulAudio(): boolean {
    return (
      chunksSentCount >= 15 ||
      peakSeen >= params.wakePeak * 0.45 ||
      partialText.value.trim().length >= 2
    )
  }

  /** 空句结束：多数情况静默回 resting，仅「确实说了但未识别」才提示。 */
  function finishEmptyCapture(reason: 'no_audio' | 'no_text') {
    partialText.value = ''
    partialUpdatedAt = 0
    clearSilenceWatch()
    stopStreamCheck()
    usePetStore().releaseVoiceBubble(0)

    const shouldPrompt =
      reason === 'no_text' &&
      hadMeaningfulAudio() &&
      Date.now() - lastEmptyPromptAt >= EMPTY_PROMPT_COOLDOWN_MS

    if (shouldPrompt) {
      lastEmptyPromptAt = Date.now()
      statusText.value = '没听到声音，请再说一次'
      usePetStore().showSpeechBubble('没听到声音，请大声一点~', 3000)
    } else if (import.meta.env.DEV && reason === 'no_text') {
      console.debug('[voice] empty capture dismissed (chunks=%d peak=%s)', chunksSentCount, peakSeen.toFixed(3))
    }

    enterResting(shouldPrompt ? undefined : 'Mochi 在休息... 说话我就听')
  }

  function detachHandler() {
    unsub?.()
    unsub = null
  }

  /** 连接已就绪时的默认状态文案（清除 connect 失败残留提示）。 */
  function defaultVoiceReadyStatus(): string {
    return recording ? 'Mochi 在休息... 说话我就听' : '输入消息或开始语音对话'
  }

  /** connect 失败后会写入超时/失败文案；连接恢复时须主动清掉，避免误报残留。 */
  function clearStaleConnectStatus() {
    const stale = new Set([
      '语音连接超时，请再点一次话筒',
      '连接失败，请关闭面板重新打开',
      '连接中...',
      '连接失败，请稍后再试',
    ])
    if (stale.has(statusText.value)) {
      statusText.value = defaultVoiceReadyStatus()
    }
  }

  /** WS 已 OPEN 时同步 store 状态，避免 connected 与 socket 不一致导致重复 connect。 */
  function syncOpenVoiceChannel() {
    connected.value = true
    if (!unsub) {
      unsub = realtimeSession.on(handleEvent)
    }
    clearStaleConnectStatus()
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
      if (owner === 'chat') {
        // 聊天窗已关但 localStorage 仍占锁 → 自动还给桌宠，避免「说话完全无反应」
        const chatVisible = await isChatWindowVisible()
        if (!chatVisible) {
          await claimVoiceOwner('pet')
          return true
        }
        return false
      }
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
  /** 休息态：流式 ASR 探名，听到名字即唤醒主人（优先于声纹误拒） */
  let nameProbeActive = false
  let nameDetectedInProbe = false
  /** 最近一次唤醒 probe 的声纹得分（供拒答策略判断）。 */
  let lastWakeProbeScore: number | null = null
  /** 对话中声纹 stream_check 定时器。 */
  let streamCheckTimer: ReturnType<typeof setInterval> | null = null
  let streamCheckGraceTimer: ReturnType<typeof setTimeout> | null = null
  /** P2：对话中人脸 probe 定时器。 */
  let faceCheckTimer: ReturnType<typeof setInterval> | null = null
  /** 最近一次人脸 probe 结果（供 identity gate 融合）。 */
  let lastFaceProbe: { match: boolean; score: number; detected: boolean } | null = null
  /** 文字聊天临时停麦前是否在语音会话中（结束后需恢复麦克风）。 */
  let voicePausedForText = false
  /** 主人 turn 内锁定 PCM 上传，stream_check 不误拦导致 ASR 空。 */
  let ownerTurnUploadLock = false
  /** 本 turn 是否已开始上传（首包语音后连续上传，保证流式 ASR 不断流）。 */
  let turnUploadStarted = false
  /** ASR partial 最近一次更新时间（眼耳协同：partial 仍在变则不提交）。 */
  let partialUpdatedAt = 0
  /** 多模态仲裁：延长等待截止时间（主人可能还在组织语言）。 */
  let thinkingHoldUntil = 0
  /** pause_probe 之后用户是否已继续说话（区分句中停顿 vs 句末结束）。 */
  let resumedAfterPauseProbe = false
  /** Tier-0 vision_pause_hint：服务端推断仍在组织语言 */
  let pauseHintComposing = false
  /** 本 turn 在途视觉/提交可 abort（barge-in / 新 wake）。 */
  let turnAbortController: AbortController | null = null
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
    // 设置页录入后写入 mochi_owner_embedding；先读本地缓存以便 API 慢/失败时仍可开聊
    const cached = readCachedVoiceprintEmbedding()
    if (cached) ownerEmbedding = cached

    try {
      const status = await getVoiceprintStatus()
      if (status.enrolled && status.embedding?.length) {
        ownerEmbedding = new Float32Array(status.embedding)
        cacheVoiceprintEmbedding(status.embedding)
      } else if (status.enrolled && cached) {
        // 服务端已录入但未返回 embedding 字段时，沿用本地缓存
        ownerEmbedding = cached
      } else if (!status.enrolled) {
        ownerEmbedding = null
        clearVoiceprintEmbeddingCache()
      }
    } catch {
      if (cached) {
        ownerEmbedding = cached
        if (import.meta.env.DEV) {
          console.warn('[voiceprint] API failed, using cached embedding')
        }
      } else {
        ownerEmbedding = null
      }
    }
  }
  /** Phase4：本地 sidecar 已移除，探测固定为不可达。 */
  async function refreshXasrSidecarProbe() {
    xasrSidecarReachable.value = false
    return false
  }

  /** Phase4：本地 sidecar 已移除，探测固定为不可达。 */
  async function refreshXttsSidecarProbe() {
    xttsSidecarReachable.value = false
    return false
  }

  /** 设置页保存 STT/TTS 模式后立即生效（Phase4 固定云端）。 */
  async function refreshVoiceBackendPrefs() {
    sttBackendLabel.value = 'cloud'
    ttsBackendLabel.value = 'cloud'
    if (realtimeSession.isOpen()) {
      await realtimeSession.sendClientCaps({ localTts: false })
    }
  }

  function mapDismissReason(reason: string): string {
    const r = reason.trim()
    if (r === 'noise_filler') return '嗯、啊等语气词，Mochi 在听但没接话'
    if (r.startsWith('gate:')) return '这句话像是自言自语或背景音，暂不回应'
    if (r === 'empty_asr') return '未识别到有效内容'
    if (r === 'empty_llm') return 'Mochi 一时想不出该怎么回'
    return '已听到，暂不需要回应'
  }

  /** 办公专注 DND：连续对话改 resting，减少打扰。 */
  function readFocusWorkDnd(): boolean {
    if (typeof window === 'undefined') return false
    return localStorage.getItem('mochi_focus_work_dnd') === '1'
  }

  /** 讨论模式：优先连续聆听与展开观点（localStorage mochi_voice_discussion_mode=1）。 */
  function readVoiceDiscussionMode(): boolean {
    if (typeof window === 'undefined') return false
    return localStorage.getItem('mochi_voice_discussion_mode') === '1'
  }

  function shouldPreferContinuousListen(): boolean {
    if (readVoiceDiscussionMode()) return true
    return !readFocusWorkDnd()
  }

  function clearPendingTurnState() {
    pendingTextTurn = null
    pendingVoiceTurn = null
  }

  /** 读取用户「默认语音回复」偏好（设置页 voice_reply_default）。 */
  async function readVoiceReplyDefault(): Promise<boolean> {
    try {
      const prefs = await getUserPreferences()
      return prefs.voice_reply_default !== false
    } catch {
      if (typeof window === 'undefined') return true
      return localStorage.getItem('mochi_voice_reply_default') !== '0'
    }
  }

  /** 一轮 TTS 播报结束（云端 tts_done 后调用）。 */
  function handleTtsTurnComplete() {
    if (presenceChatListenAfterTts) {
      presenceChatListenAfterTts = false
      clearTtsWatchdog()
      ttsPlayer.markDone(() => {
        resetTurnTiming()
        ttsPlayer.resetTurn()
        enterContinuousListen('我在这儿，你说~')
        usePetStore().syncAnimationFromState()
      })
      return
    }
    if (phase !== 'resting' && phase !== 'idle') {
      finishTextTurn()
    } else {
      clearTtsWatchdog()
      textSending = false
      resetTurnTiming()
    }
  }

  /** 诊断：本地 TTS 已移除（Phase4），保留 API 供设置页/诊断面板调用。 */
  async function testLocalTts(_sample?: string): Promise<boolean> {
    statusText.value = '本地 TTS 已迁移至云端（Phase4）'
    return false
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
    if (!FACE_RECOGNITION_ENABLED) return false
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

  /** 唤醒前快拍验脸：需服务端开启视觉；否则仅声纹。 */
  async function quickFaceProbeForWake(): Promise<{ match: boolean; score: number; detected: boolean } | null> {
    if (!faceprintReady() || !getClientConfig().visionEnabled) return null
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
    // 已录入 embedding 即可开聊；CAM++ 模型加载失败时 verify 会 fail-open，不应挡入口
    return !!ownerEmbedding?.length
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

  /** P2：周期性从会话摄像头抓拍并验脸（低配模式关闭以省 CPU）。 */
  function startFaceCheck() {
    stopFaceCheck()
    const fp = getFaceprintConfig()
    if (
      !shouldStartPeriodicFaceCheck({
        faceprintEnabled: fp.enabled,
        faceprintReady: faceprintReady(),
        phase,
        recording,
      })
    ) {
      return
    }
    const interval = getPeriodicFaceCheckIntervalMs()
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

  /** 同步 DEV 面板用的 Orchestrator 相位。 */
  function syncPerceptionPhaseRef() {
    perceptionPhase.value = perception.getPerceptionPhase()
  }

  /** Phase E：眼耳感知编排（Turn Phase FSM + Eye/Ear Channel）。 */
  const perception = createPerceptionOrchestrator({
    getAbortSignal: () => turnAbortController?.signal,
    sendVisionFrame: (b64, meta) =>
      realtimeSession.sendVisionFrame(b64, {
        reason: meta.reason,
        faceProbe: meta.faceProbe,
        partialText: meta.partialText,
      }),
    probeFaceFromJPEG,
    faceprintReady,
    runPeriodicFaceCheck: () => runFaceCheck(),
    onEvent: () => syncPerceptionPhaseRef(),
  })

  function buildEarTurnState() {
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
      pauseHintComposing,
      sttMode: effectiveSttMode,
      speechEndedAt: speechEndedAt > 0 ? speechEndedAt : undefined,
    }
  }

  /** 对话中周期性验声纹（P0 stream_check）+ 人脸融合（P2）。 */
  function startStreamCheck() {
    stopStreamCheck()
    const vp = getVoiceprintConfig()
    if (
      !shouldStartStreamCheck({
        voiceprintRequired: vp.required,
        voiceprintReady: voiceprintReady(),
        sttMode: effectiveSttMode,
      })
    ) {
      return
    }
    const interval = getStreamCheckIntervalMs()
    const graceMs = getStreamCheckGraceMs()
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
    if (probe === 'ok') {
      endNameWakeProbe(false)
      return
    }
    // VAD 已报 speech_start 但声纹/分类器误拒：在能量与人声明显时 fallback（避免声纹微小抖动导致主人被当陌生人无视）
    const isSpeaking = speechVad?.isSpeaking() ?? false
    if (
      (probe === 'not_speech' && micLevel.value >= params.wakePeak * 2.0 && isSpeaking) ||
      (probe === 'not_owner' && micLevel.value >= params.wakePeak * 1.3 && isSpeaking && (lastWakeProbeScore ?? 0) >= 0.22)
    ) {
      if (import.meta.env.DEV) {
        console.debug('[voiceprint] vad energy/score wake fallback peak=%s score=%s', micLevel.value.toFixed(3), lastWakeProbeScore?.toFixed(3) ?? 'null')
      }
      identityGate.markOwnerMatch()
      endNameWakeProbe(false)
      wakeOnSpeech()
      return
    }
    // 声纹极低分（如背景杂音）才静默忽略
    if (probe === 'not_owner' && import.meta.env.DEV) {
      console.debug('[voiceprint] wake silent reject score=%s', lastWakeProbeScore?.toFixed(3) ?? 'null')
    }
  }

  /** 休息态检测到说话：并行启动「听名字」与声纹唤醒。 */
  function startNameWakeProbe() {
    if (phase !== 'resting' || !recording || nameProbeActive) return
    nameProbeActive = true
    nameDetectedInProbe = false
    turnUploadStarted = true

    if (!realtimeSession.isOpen()) {
      nameProbeActive = false
      turnUploadStarted = false
      return
    }
    if (shouldUploadPcmToServer()) {
      sendUtteranceAudioStart(true)
    }
    flushPreRollAudio()

    if (import.meta.env.DEV) {
      console.debug('[name_wake] probe started backend=cloud')
    }
  }

  function endNameWakeProbe(cancel: boolean) {
    if (!nameProbeActive) return
    nameProbeActive = false
    if (cancel && phase === 'resting') {
      turnUploadStarted = false
      realtimeSession.sendUtteranceCancel()
      if (import.meta.env.DEV) {
        console.debug('[name_wake] probe cancelled (no name)')
      }
    }
  }

  /**
   * 名字探针 speech_end：有有效 partial + 主人声纹/能量 → 唤醒并提交；
   * 否则丢弃，避免 resting 下 partial 悬挂不提交。
   */
  async function finishNameProbeOnSpeechEnd() {
    if (!nameProbeActive || phase !== 'resting') return
    if (nameDetectedInProbe) return

    const partial = partialText.value.trim()
    const hasAudio = chunksSentCount > 30 || peakSeen >= params.wakePeak * 0.5
    const ownerOk = hasAudio ? await verifyOwnerBeforeSubmit() : false
    const hasContent = partial.length >= 2

    if (
      hasContent &&
      hasAudio &&
      (ownerOk || peakSeen >= params.wakePeak || micLevel.value >= params.wakePeak)
    ) {
      if (import.meta.env.DEV) {
        console.debug('[name_wake] probe promote+submit partial=%d chars', partial.length)
      }
      endNameWakeProbe(false)
      promoteNameProbeToWake()
      await submitUtteranceAsync(true)
      return
    }

    endNameWakeProbe(true)
    partialText.value = ''
    partialUpdatedAt = 0
    peakSeen = 0
    heardSpeech = false
    usePetStore().releaseVoiceBubble(0)
  }

  /** 流式 ASR 已听到名字：升级为正式聆听，不再二次 audio_start。 */
  function promoteNameProbeToWake() {
    if (phase !== 'resting' || !recording) return
    nameProbeActive = false
    nameDetectedInProbe = true
    identityGate.markOwnerMatch()
    usePetStore().clearEmotionHold()
    setPhase('user_speaking')
    uploadSeq = uploadSeq || 0
    heardSpeech = true
    touchLastSpeechAt()
    utteranceStartedAt = Date.now()
    ambientPresence.setOwnerSpeaking(true)
    identityGate.resetTurn()
    ownerTurnUploadLock = true
    turnUploadStarted = true
    perception.onSpeechStart()
    startStreamCheck()
    startSilenceWatch()
    statusText.value = '正在听...'
    if (import.meta.env.DEV) {
      console.debug('[name_wake] promoted to user_speaking')
    }
  }

  async function beginRestingSpeechWake() {
    if (phase !== 'resting' || !recording) return
    // 先验声纹唤醒；成功则无需名字探针
    await tryWakeFromResting()
    if (phase !== 'resting') return

    if (!realtimeSession.isOpen()) {
      await connectIfOwner()
    }
    if (realtimeSession.isOpen()) {
      startNameWakeProbe()
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
    const trimmed = stripMoodTags(text.trim())
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

  /** Phase4：固定云端 STT，PCM 始终上传服务端。 */
  function shouldUploadPcmToServer(): boolean {
    return true
  }

  /** 本 utterance 是否已向服务端发送 audio_start（防重复 reset ASR）。 */
  let utteranceAudioStartSent = false

  function resetUtteranceAudioStart() {
    utteranceAudioStartSent = false
  }

  /** 发送 audio_start：播放/思考期与重复 utterance 内 dedup。 */
  function sendUtteranceAudioStart(force = false): boolean {
    if (!force && utteranceAudioStartSent) return false
    if (phase === 'agent_speaking' || phase === 'processing') return false
    if (!shouldUploadPcmToServer() || !realtimeSession.isOpen()) return false
    utteranceAudioStartSent = true
    return realtimeSession.sendAudioStart()
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
    clearPendingTurnState()
    textViaRest = false
    voiceViaRest = false
    const finalReply = replyText.value.trim()
    const hadVoice = ttsPlayer.hadPlayback
    if (finalReply) {
      syncReplyBubble(finalReply)
    }
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
          enterResting(hint)
        } else if (!hadVoice && finalReply) {
          hint = isOpusDecodeSupported()
            ? '文字已回复，语音播放失败，请检查音量'
            : '文字已回复（当前环境不支持 Opus，已请求 MP3）'
          if (shouldPreferContinuousListen()) {
            enterContinuousListen(hint)
          } else {
            enterResting(hint)
          }
        } else if (shouldPreferContinuousListen()) {
          // 正常一轮对话结束：保持聆听，避免「说一句停一句」的问答机感
          enterContinuousListen()
        } else {
          enterResting('专注模式：需要时请叫我')
        }
        usePetStore().syncAnimationFromState()
      })
    } else {
      usePetStore().releaseVoiceBubble(HEARD_BUBBLE_GRACE_MS)
      resetTurnTiming()
      const resumeVoice = voicePausedForText
      if (resumeVoice) {
        voicePausedForText = false
      }
      ttsPlayer.markDone(async () => {
        if (resumeVoice) {
          await resumeVoiceAfterText()
        } else {
          setPhase('idle')
          statusText.value = '输入消息或开始语音对话'
        }
        usePetStore().syncAnimationFromState()
      })
    }
  }

  /** 文字回合结束后恢复云端语音采音（sendTextMessage 会 pauseVoiceForText）。 */
  async function resumeVoiceAfterText() {
    if (!realtimeSession.isOpen()) {
      await connectIfOwner()
    }
    if (!realtimeSession.isOpen()) {
      statusText.value = '语音连接未就绪，请点话筒重新开始'
      setPhase('idle')
      return
    }
    try {
      await initVad()
      partialText.value = ''
      replyText.value = ''
      ttsPlayer.stop()
      clearTtsWatchdog()
      submitLock = false
      micLevel.value = 0
      recording = true
      talking.value = true
      await startCloudTalk()
    } catch {
      recording = false
      talking.value = false
      setPhase('idle')
      statusText.value = '无法恢复麦克风，请点话筒重新开始'
    }
  }

  function clearSilenceWatch() {
    if (silenceTimer) {
      clearInterval(silenceTimer)
      silenceTimer = null
    }
  }

  function clearTtsWatchdog() {
    if (ttsWatchdog) {
      clearTimeout(ttsWatchdog)
      ttsWatchdog = null
    }
  }

  function setPhase(next: TurnPhase) {
    phase = next
    turnPhase.value = next
    perception.syncTurnPhase(next)
    syncPerceptionPhaseRef()
    resting.value = next === 'resting'
    setProcessing(next === 'processing' || next === 'agent_speaking')
    speechVad?.setPlaybackMode(next === 'agent_speaking')
  }

  function refreshRuntimeParams() {
    params = defaultRuntimeParams()
  }

  /** 云端路径：客户端识别结果以 text_input 提交（无 turn_pcm）。 */
  function submitVoiceText(text: string) {
    const trimmed = text.trim()
    if (!trimmed || submitLock) return
    if (!realtimeSession.isOpen()) {
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }

    clearSilenceWatch()
    submitLock = true
    setPhase('processing')
    serverSessionState.value = 'thinking'
    heardSpeech = false
    partialText.value = ''
    statusText.value = '处理中...'
    turnStartAt = Date.now()
    playbackMarked = false

    commitUserMessage(trimmed, 'voice')
    pendingVoiceTurn = trimmed
    replyText.value = ''
    startTtsWatchdog()

    resetTurnAbort()
    const sent = realtimeSession.sendTextInput(trimmed, { voiceReply: true })
    void perception.prepareBeforeSubmit(trimmed)
    if (!sent) {
      submitLock = false
      pendingVoiceTurn = null
      messages.value.pop()
      enterResting()
      statusText.value = '发送失败，请重试'
      return
    }
    setTimeout(() => {
      submitLock = false
    }, 800)
  }

  function buildTurnEndSignals() {
    return perception.buildTurnEndSignals(buildEarTurnState())
  }

  /** THINK 阶段 Tier-0 GLANCE：仅 faceDet，不触发 Tier-1 VL。 */
  function runThinkGlance() {
    perception.runThinkGlance()
  }

  /** 静音≥3s：眼抓拍 + Tier-0 pause_hint 回传 turnEndArbiter */
  async function runPauseThinkingProbe() {
    const partial = partialText.value.trim()
    await perception.runPauseProbe(partial)
    const decision = evaluateTurnEnd(buildTurnEndSignals())
    if (decision.extendHoldMs) {
      thinkingHoldUntil = Math.max(thinkingHoldUntil, Date.now() + decision.extendHoldMs)
      if (import.meta.env.DEV) {
        console.debug('[turn_end] pause_probe hold=%sms reason=%s', decision.extendHoldMs, decision.reason)
      }
    }
  }

  function startSilenceWatch() {
    clearSilenceWatch()
    silenceTimer = setInterval(() => {
      if (!recording || phase !== 'user_speaking') return
      const signals = buildTurnEndSignals()
      const silence = Date.now() - signals.lastSpeechAt

      if (silence >= PAUSE_PROBE_MS && !signals.disablePauseProbe && !perception.isPauseProbeDone() && signals.heardSpeech) {
        perception.markPauseProbeDone()
        void runPauseThinkingProbe()
      }

      const decision = evaluateTurnEnd(signals)

      if (decision.ready) {
        void submitUtterance()
      }
      if (utteranceStartedAt > 0 && Date.now() - utteranceStartedAt >= MAX_UTTERANCE_MS) {
        if (hadMeaningfulAudio() || partialText.value.trim()) {
          void submitUtterance(true)
        } else {
          finishEmptyCapture('no_audio')
        }
      }
    }, 200)
  }

  function setProcessing(v: boolean) {
    processingRef.value = v
  }

  /**
   * 经 /ws/voice speak_only 播报短句（唤醒问候、提醒等），统一云端 TTS。
   * @returns 是否已成功发送 speak_only
   */
  async function speakCloudOnly(
    text: string,
    opts?: { ensureTalk?: boolean; listenAfter?: boolean },
  ): Promise<boolean> {
    const trimmed = text.trim()
    if (!trimmed || !cloudVoiceEnabled()) return false

    if (opts?.ensureTalk && !recording) {
      const ok = await startTalk()
      if (!ok) return false
    }

    if (!realtimeSession.isOpen()) return false

    stopActiveTtsForCloudSpeak()

    if (opts?.listenAfter) {
      presenceChatListenAfterTts = true
    }

    replyText.value = trimmed
    syncReplyBubble(trimmed)
    setPhase('agent_speaking')
    serverSessionState.value = 'speaking'
    statusText.value = opts?.listenAfter ? 'Mochi 想跟你说说话~' : 'Mochi 正在说话...'
    startTtsWatchdog()

    if (!realtimeSession.sendSpeakOnly(trimmed)) {
      if (opts?.listenAfter) presenceChatListenAfterTts = false
      return false
    }
    return true
  }

  /** 桌宠唤醒后云端朗读「在呢，主人！」。 */
  function speakWakeGreeting(): void {
    void speakCloudOnly(getWakeGreetingText())
  }

  /** 在场闲聊送达：Mochi 声线播报并进入可接话状态。 */
  async function deliverPresenceChat(message: string, animation?: string) {
    const trimmed = message.trim()
    if (!trimmed || wasProactiveRecentlyShown(trimmed)) return

    handleProactiveMessage({ message: trimmed, animation }, { priority: true })
    commitAssistantMessage(trimmed)

    await speakCloudOnly(trimmed, { ensureTalk: true, listenAfter: true })
  }

  function enterContinuousListen(hint?: string) {
    clearTtsWatchdog()
    stopStreamCheck()
    identityGate.resetTurn()
    perception.resetTurn()
    submitLock = false
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    heardSpeech = false
    lastSpeechAt = 0
    lastSpeechAtMs.value = 0
    utteranceStartedAt = 0
    speechEndedAt = 0
    bargeAccumMs = 0
    wakeAccumMs = 0
    ttsStartedAt = 0
    partialText.value = ''
    partialUpdatedAt = 0
    resumedAfterPauseProbe = false
    pauseHintComposing = false
    thinkingHoldUntil = 0
    resetTurnTiming()
    chunksSent.value = 0
    resetUtteranceAudioStart()
    // 保留 pcm ring，连续对话时 pre-roll 仍有效

    setPhase('user_speaking')
    identityGate.markOwnerMatch()
    // 连续对话：等同手动唤醒，免等 stream_check 才开 PCM
    ownerTurnUploadLock = true
    ambientPresence.setOwnerSpeaking(true)
    statusText.value = hint ?? '还在呢，继续说~'
    usePetStore().setAnimation('happy')

    partialText.value = ''
    partialUpdatedAt = 0
    speechVad?.reset()

    const echoDelayMs = Math.max(1200, params.echoGuardMs)

    setTimeout(() => {
      if (!recording || phase !== 'user_speaking') return
      turnUploadStarted = true
      utteranceStartedAt = Date.now()
      if (shouldUploadPcmToServer()) {
        sendUtteranceAudioStart(true)
      }
      startStreamCheck()
      startSilenceWatch()
    }, echoDelayMs)
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
    lastSpeechAtMs.value = 0
    utteranceStartedAt = 0
    speechEndedAt = 0
    bargeAccumMs = 0
    wakeAccumMs = 0
    nameProbeActive = false
    nameDetectedInProbe = false
    turnUploadStarted = false
    ttsStartedAt = 0
    partialText.value = ''
    partialUpdatedAt = 0
    perception.resetTurn()
    lastFaceProbe = null
    resetTurnTiming()
    chunksSent.value = 0
    resetPcmRing()
    speechVad?.reset()
    resetUtteranceAudioStart()
    ambientPresence.setOwnerSpeaking(false)
    if (recording) {
      statusText.value = hint ?? 'Mochi 在休息... 说话我就听'
      usePetStore().setAnimation('idle')
      startSilenceWatch()
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
      if (shouldUploadPcmToServer()) {
        realtimeSession.sendAudio(arrayBufferToBase64(boosted), uploadSeq)
      }
    }
  }

  /** Owner started speaking — wake up and begin uploading. */
  function wakeOnSpeech() {
    resetTurnAbort()
    if (phase !== 'resting' || !recording) return
    usePetStore().clearEmotionHold()
    setPhase('user_speaking')
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    // 等检测到真实语音后再标记 heardSpeech，避免点击唤醒后静音误触发提交
    heardSpeech = false
    lastSpeechAt = 0
    lastSpeechAtMs.value = 0
    utteranceStartedAt = Date.now()
    perception.resetTurn()
    // 保留唤醒时的人脸 probe，供 stream_check 立即融合（勿清空）
    chunksSent.value = 0
    partialText.value = ''
    partialUpdatedAt = 0
    resumedAfterPauseProbe = false
    pauseHintComposing = false
    thinkingHoldUntil = 0
    ambientPresence.setOwnerSpeaking(true)
    identityGate.markOwnerMatch()
    identityGate.resetTurn()
    ownerTurnUploadLock = true
    // 唤醒即连续上传（含点击 manual wake），避免 VAD 已触发但 PCM 未传导致 ASR 空
    turnUploadStarted = true
    resetUtteranceAudioStart()
    if (shouldUploadPcmToServer()) {
      sendUtteranceAudioStart(true)
      flushPreRollAudio()
    } else {
      flushPreRollAudio()
    }
    perception.onSpeechStart()
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

  /** 桌宠单击：仅当 turn-end 已就绪时才 force 提交，避免误触截断长句。 */
  function trySubmitFromTap(): boolean {
    if (phase !== 'user_speaking') return false
    if (!evaluateTurnEnd(buildTurnEndSignals()).ready) return false
    void submitUtterance(true)
    return true
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
      // 无 ASR partial 不提交（手动 force 除外），避免服务端空识别
      if (partialText.value.trim().length < 2) {
        if (chunksSentCount > 50) {
          finishEmptyCapture('no_text')
        }
        return
      }
    }

    if (chunksSentCount === 0) {
      if (partialText.value.trim()) {
        commitHeardText(partialText.value, { dismissed: true, dismissReason: '未检测到有效语音' })
      }
      finishEmptyCapture('no_audio')
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
    // 等服务端 ASR final 后再 turn_ack → processing；避免尾音仍在识别时显示「正在想」
    statusText.value = '收尾聆听...'
    heardSpeech = false
    lastSpeechAt = 0
    lastSpeechAtMs.value = 0
    utteranceStartedAt = 0
    speechEndedAt = 0
    bargeAccumMs = 0
    speechVad?.reset()
    turnStartAt = Date.now()
    playbackMarked = false
    stopPlaybackBeforeSubmit()
    ttsPlayer.resetTurn()

    resetTurnAbort()
    await perception.prepareBeforeSubmit(partialText.value.trim())

    if (turnAbortController?.signal.aborted) {
      submitLock = false
      enterResting()
      return
    }

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
      void beginRestingSpeechWake()
      return
    }

    if (phase === 'resting' && nameProbeActive && ev === 'speech_end') {
      if (!nameDetectedInProbe) {
        void finishNameProbeOnSpeechEnd()
      }
      return
    }

    if (phase !== 'user_speaking') return

    if (ev === 'speech_start') {
      heardSpeech = true
      speechEndedAt = 0
      touchLastSpeechAt()
      // 句中 3s 停顿后续说：解除 thinking hold，句末静音走正常仲裁
      if (perception.isPauseProbeDone()) {
        resumedAfterPauseProbe = true
        thinkingHoldUntil = 0
      }
      statusText.value = '正在听...'
      perception.onSpeechStart()
      return
    }

    if (ev === 'speech_end') {
      if (phase === 'user_speaking') {
        speechEndedAt = Date.now()
        // 句末检测与上传门控分离：有音量/partial 即视为说完，避免声纹门控导致永不提交
        if (peakSeen >= 0.03 || partialText.value.trim()) {
          heardSpeech = true
          touchLastSpeechAt()
        }
      }
      return
    }
  }

  function applyTtsEpoch(epoch?: number) {
    if (epoch != null && epoch > 0) {
      ttsPlayer.setPlaybackEpoch(epoch)
      if (import.meta.env.DEV) {
        console.info('[realtime] tts_epoch=%d', epoch)
      }
    } else {
      ttsPlayer.bumpPlaybackEpoch()
    }
  }

  function bargeIn() {
    if (phase !== 'agent_speaking') return
    if (!ttsPlayer.hadPlayback && !replyText.value.trim()) return
    cancelTurnAbort()
    ttsPlayer.bumpPlaybackEpoch()
    stopActiveTtsForCloudSpeak()
    clearTtsWatchdog()
    replyText.value = ''
    usePetStore().releaseVoiceBubble(0)
    realtimeSession.sendInterrupt()
    // 与正常轮次结束对齐：打断后可直接接下一句
    enterContinuousListen('你说~')
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
    const vadCfg = getRealtimeConfig().vad
    speechVad = new HybridSpeechVad(handleVadEvent, vadCfg)
    await speechVad.init()
  }

  async function pauseVoiceForText() {
    if (!recording) return
    voicePausedForText = true
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
        replyText.value = reply
      }
      finishTextTurn()
    } catch (e) {
      textViaRest = false
      voiceViaRest = false
      textSending = false
      clearPendingTurnState()
      clearTtsWatchdog()
      setPhase('idle')
      const last = messages.value[messages.value.length - 1]
      if (last?.role === 'user' && last.content === trimmed) {
        messages.value.pop()
      }
      statusText.value = e instanceof Error ? e.message : '发送失败，请重试'
    }
  }

  /** 语音轮次 WS 断线：REST 拉 LLM + 本地 TTS（与 sendTextViaRest 类似）。 */
  async function sendVoiceViaRest(trimmed: string) {
    if (voiceViaRest || textViaRest) return
    voiceViaRest = true
    pendingVoiceTurn = null
    clearTurnAckWait()
    statusText.value = '连接断开，改用文字通道...'
    replyText.value = ''
    try {
      const reply = await streamChatMessage(trimmed, (token) => {
        replyText.value += token
        syncReplyBubble(replyText.value)
      })
      if (reply) {
        commitAssistantMessage(reply)
        replyText.value = reply
      }
      finishTextTurn()
    } catch (e) {
      voiceViaRest = false
      clearTtsWatchdog()
      enterResting('连接中断，请再说一次')
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

    // 弹窗 chat 与 pet 分 WebView，发送前同步 token
    const { useAuthStore } = await import('@/stores/authStore')
    useAuthStore().syncFromStorage()

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

    const voiceReply = await readVoiceReplyDefault()
    const ackWait = beginTurnAckWait(TEXT_TURN_ACK_MS)
    const sent = realtimeSession.sendTextInput(
      trimmed,
      voiceReply ? { voiceReply: true } : undefined,
    )
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
    ensureEventLoopProbe()
    if (!(await shouldOwnVoice())) {
      const owner = getStoredVoiceOwner()
      if (voiceWindow === 'pet' && owner === 'chat') {
        console.warn('[realtime] connect blocked: chat window owns voice WS')
        statusText.value = '聊天窗口占用语音连接，请先关闭聊天'
      } else if (voiceWindow === 'chat' && owner !== 'chat') {
        console.warn('[realtime] connect blocked: voice owner=%s expected chat', owner)
        statusText.value = '语音通道未就绪，请关闭聊天后重新打开'
      } else {
        console.warn('[realtime] connect blocked: voiceWindow=%s owner=%s', voiceWindow, owner)
        statusText.value = '连接失败，请稍后再试'
      }
      return
    }
    if (realtimeSession.isOpen()) {
      syncOpenVoiceChannel()
      return
    }

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
      await realtimeSession.sendClientCaps({ localTts: false })
    } catch (e) {
      connected.value = false
      detachHandler()
      console.warn('[realtime] /ws/voice connect failed', e)
      const msg = e instanceof Error ? e.message : String(e)
      if (msg.includes('timeout') || msg.includes('closed before open')) {
        statusText.value = '语音连接超时，请再点一次话筒'
      } else {
        statusText.value = '连接失败，请关闭面板重新打开'
      }
      return
    }
    connected.value = true
    realtimeSession.sendPrewarm()
    statusText.value = defaultVoiceReadyStatus()
  }

  async function connect(): Promise<void> {
    if (connectFlight) return connectFlight
    const gen = ++connectGeneration
    connectFlight = connectInternal()
      .finally(() => {
        if (connectGeneration === gen) {
          connectFlight = null
        }
      })
    return connectFlight
  }

  async function reconnectAfterDisconnect(restoreVoice: boolean) {
    if (reconnecting) return
    reconnecting = true
    statusText.value = '连接断开，正在重连...'
    try {
      await connectIfOwner()
      if (connected.value && realtimeSession.isOpen()) {
        clearStaleConnectStatus()
      }
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
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        await connect()
        if (realtimeSession.isOpen()) {
          console.info('[realtime] push voice WS ready')
          return
        }
      } catch (e) {
        console.warn('[realtime] push connect attempt failed', attempt + 1, e)
      }
      await new Promise((r) => setTimeout(r, 800 * (attempt + 1)))
    }
    console.warn('[realtime] push voice WS not ready after retries')
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
          // 名字探针：休息态也上传 PCM，供流式 ASR 识别「卡卡」等称呼
          if (nameProbeActive && turnUploadStarted) {
            uploadSeq++
            chunksSentCount++
            chunksSent.value = chunksSentCount
            if (shouldUploadPcmToServer()) {
              realtimeSession.sendAudio(arrayBufferToBase64(boosted), uploadSeq)
            }
          }
          // 休息态：须 Silero/能量 VAD 认为在说话，或峰值极高，才尝试唤醒
          if (vadSpeaking && peak >= params.wakePeak * 0.85) {
            wakeAccumMs += 20
            if (wakeAccumMs >= WAKE_CONFIRM_MS) {
              wakeAccumMs = 0
              void tryWakeFromResting()
            }
          } else if (peak >= params.wakePeak * 2.8) {
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
              touchLastSpeechAt()
              // VAD 未切 speech_start 时，能量续说同样解除 pause hold
              if (perception.isPauseProbeDone() && !resumedAfterPauseProbe) {
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
          if (shouldUploadPcmToServer()) {
            realtimeSession.sendAudio(arrayBufferToBase64(boosted), uploadSeq)
          }
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
    enterContinuousListen('你说~')
  }

  async function startTalk(): Promise<boolean> {
    if (recording) return true

    await initClientConfig().catch(() => {})
    refreshRuntimeParams()

    // Phase：语音优先，面容/摄像头预热延后（FACE_RECOGNITION_ENABLED=false）
    const visionWarmPromise =
      FACE_RECOGNITION_ENABLED && getClientConfig().visionEnabled
        ? prewarmVisionSession()
        : Promise.resolve()

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

    sttBackendLabel.value = 'cloud'
    ttsBackendLabel.value = 'cloud'
    if (realtimeSession.isOpen()) {
      await realtimeSession.sendClientCaps({ localTts: false })
    }

    await speakerVerifier.init().catch(() => {})
    if (FACE_RECOGNITION_ENABLED) {
      await faceVerifier.init().catch(() => {})
      await loadOwnerFaceprint()
    }
    await soundClassifier.init().catch(() => {})
    await loadOwnerVoiceprint()

    if (!voiceprintReady()) {
      statusText.value = '请先在设置中录入主人声纹（设置 → 主人声纹）'
      sttBackendLabel.value = ''
      return false
    }

    await pauseAmbientMicForTalk()

    partialText.value = ''
    replyText.value = ''
    ttsPlayer.stop()
    clearTtsWatchdog()
    submitLock = false
    micLevel.value = 0

    // 语音会话依赖麦克风；提前验证，避免 X-ASR/WS 白跑后 silent fail
    {
      const { ensureTauriMicrophoneAccess, micPermissionDeniedMessage, warmUpMicrophoneAccess } =
        await import('@/utils/micPermission')
      const micOk = isTauri()
        ? await ensureTauriMicrophoneAccess()
        : await warmUpMicrophoneAccess()
      if (!micOk) {
        statusText.value = micPermissionDeniedMessage()
        return false
      }
    }

    try {
      await startCloudTalk()
      await visionWarmPromise.catch(() => {})
      return recording
    } catch (e) {
      const err = e as DOMException
      if (err?.name === 'NotAllowedError') {
        const { micPermissionDeniedMessage } = await import('@/utils/micPermission')
        statusText.value = micPermissionDeniedMessage()
      } else if (err?.name === 'NotFoundError') {
        statusText.value = '未检测到麦克风设备'
      } else if (!statusText.value) {
        statusText.value = '无法启动麦克风'
      }
      talking.value = false
      recording = false
      setPhase('idle')
      resting.value = false
      return false
    }
  }

  async function initAmbientPresence() {
    await initClientConfig().catch(() => {})
    void refreshXasrSidecarProbe()
    await speakerVerifier.init().catch(() => {})
    await loadOwnerVoiceprint()
    // Phase D #12：ambient 阶段后台预热摄像头
    void prewarmVisionSession()
    await startAmbientPresenceService(ownerEmbedding)
  }

  async function endConversation() {
    cancelTurnAbort()
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
    sttBackendLabel.value = ''
    ttsBackendLabel.value = ''
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
        perception.trackObjectIntentFromPartial(ev.text)
        // 呼喊名字 → 立即唤醒并强制放行本 turn（服务端 gate fastpath:pet_name）
        if (textContainsPetName(ev.text)) {
          nameDetectedInProbe = true
          identityGate.markOwnerMatch()
          if (phase === 'resting' && nameProbeActive) {
            promoteNameProbeToWake()
          } else if (phase === 'resting') {
            wakeOnSpeech()
          }
        }
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
        if (textViaRest || voiceViaRest) break
        if (recording && phase === 'processing') {
          statusText.value = 'Mochi 正在回复...'
        }
        replyText.value = stripMoodTags(replyText.value + ev.token)
        syncReplyBubble(replyText.value)
        break
      case 'llm_done':
        if (textViaRest || voiceViaRest) break
        pendingVoiceTurn = null
        replyText.value = stripMoodTags(ev.text)
        syncReplyBubble(replyText.value)
        if (!presenceChatListenAfterTts) {
          commitAssistantMessage(ev.text)
        }
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
        serverSessionState.value = 'speaking'
        if (!ttsStartedAt) ttsStartedAt = Date.now()
        statusText.value = 'Mochi 正在说话...（大声说话可打断）'
        ttsPlayer.enqueue(audio, ev.format, markPlaybackStart, ev.seq, ev.ttsEpoch)
        break
      }
      case 'tts_segment_done':
        ttsPlayer.flushSegment()
        break
      case 'tts_done':
        serverSessionState.value = 'listening'
        handleTtsTurnComplete()
        break
      case 'turn_dismiss': {
        serverSessionState.value = 'listening'
        const heard = partialText.value.trim()
        if (heard) {
          commitHeardText(heard, {
            dismissed: true,
            dismissReason: mapDismissReason(ev.reason),
          })
        }
        statusText.value = mapDismissReason(ev.reason)
        break
      }
      case 'turn_metrics':
        lastTurnMetrics.value = ev.metrics
        recordTurnMetricsBaseline(ev.metrics)
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
      case 'vision_pause_hint':
        pauseHintComposing = ev.composing || isComposingExpression(ev.expression)
        if (pauseHintComposing && phase === 'user_speaking') {
          thinkingHoldUntil = Math.max(
            thinkingHoldUntil,
            Date.now() + THINKING_HOLD_EXTEND_MS,
          )
          if (import.meta.env.DEV) {
            console.debug(
              '[turn_end] vision_pause_hint expr=%s composing=%s',
              ev.expression,
              pauseHintComposing,
            )
          }
        }
        break
      case 'turn_ack':
        serverSessionState.value = 'thinking'
        signalTurnAck()
        if (turnStartAt <= 0) turnStartAt = Date.now()
        setPhase('processing')
        startTtsWatchdog()
        statusText.value = 'Mochi 正在想...'
        void runThinkGlance()
        break
      case 'interrupted':
        cancelTurnAbort()
        applyTtsEpoch(ev.ttsEpoch)
        clearTtsWatchdog()
        textSending = false
        clearPendingTurnState()
        voiceViaRest = false
        pet.releaseVoiceBubble(0)
        enterResting()
        pet.setAnimation('happy')
        break
      case 'animation':
        serverSessionState.value = ev.state
        // 会话阶段动画不覆盖 emotion FSM hold（worried/sad 等）
        if (ev.state === 'listening') {
          if (!pet.isEmotionHoldActive()) {
            pet.setAnimation('happy')
          }
        } else if (ev.state === 'idle') {
          pet.syncAnimationFromState()
        }
        break
      case 'error':
        ttsPlayer.stop()
        clearTtsWatchdog()
        textSending = false
        clearPendingTurnState()
        voiceViaRest = false
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
          statusText.value = 'Mochi 暂时想不出回复，请换个说法试试'
          if (!replyText.value.trim()) {
            commitAssistantMessage('嗯…我一时想不出怎么回，你换个说法再试试？')
          }
          if (recording) {
            enterResting()
          } else {
            finishTextTurn()
          }
          break
        }
        if (ev.code === 'ASR_NOT_CONFIGURED' || ev.code === 'ASR_FAILED') {
          const msg = ev.message
          if (recording) {
            enterResting()
            statusText.value = msg + '（可以继续说）'
          } else {
            setPhase('idle')
            statusText.value = msg
          }
          usePetStore().showSpeechBubble(msg, 8000)
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
        if (ev.source === 'presence_chat') {
          void deliverPresenceChat(ev.message, ev.animation)
          break
        }
        interruptForReminder()
        handleProactiveMessage(
          { message: ev.message, animation: ev.animation },
          { priority: true },
        )
        commitAssistantMessage(ev.message)
        if (recording && realtimeSession.isOpen()) {
          void speakCloudOnly(ev.message)
        }
        break
      case 'disconnected':
        resetConnectFlight('ws disconnected')
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

        if (pendingVoiceTurn && recording && phase === 'processing') {
          clearTtsWatchdog()
          detachHandler()
          void sendVoiceViaRest(pendingVoiceTurn)
          void connectIfOwner().catch(() => {})
          break
        }

        clearTtsWatchdog()
        textSending = false
        clearPendingTurnState()
        voiceViaRest = false
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
    sttBackendLabel,
    ttsBackendLabel,
    xasrSidecarReachable,
    xttsSidecarReachable,
    refreshXasrSidecarProbe,
    refreshXttsSidecarProbe,
    refreshVoiceBackendPrefs,
    testLocalTts,
    submitVoiceText,
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
    deliverPresenceChat,
    speakCloudOnly,
    speakWakeGreeting,
    appendAssistantMessage: commitAssistantMessage,
    interruptForReminder,
    submitUtterance,
    trySubmitFromTap,
    endConversation,
    stopTalk: submitUtterance,
    lastTurnMetrics,
    turnPhase,
    perceptionPhase,
    serverSessionState,
    lastSpeechAtMs,
    eventLoopLagMs,
  }
})
