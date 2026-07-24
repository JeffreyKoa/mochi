import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { PCMCapture, arrayBufferToBase64, pcmPeakLevel, amplifyPCM } from '@/services/pcmCapture'
import { realtimeSession, type RealtimeEvent } from '@/services/realtimeSession'
import { usePetStore } from '@/stores/petStore'
import { TTSAudioQueue } from '@/services/ttsAudioPlayer'
import { HybridSpeechVad, pcmToFloat, type VADEvent } from '@/services/sileroSpeechVad'
import { LocalSTT, isLocalSttSupported } from '@/services/localStt'
import { getRealtimeConfig, initClientConfig, resolveSttMode } from '@/config'
import { handleProactiveMessage } from '@/services/proactiveHandler'
import { streamChatMessage } from '@/services/api'

/**
 * Turn phases — like talking to a person:
 * resting: mic monitors locally, Mochi sleeps, no upload
 * user_speaking: owner voice detected, recording & uploading
 * processing / agent_speaking: Mochi thinks & replies
 */
type TurnPhase = 'idle' | 'resting' | 'user_speaking' | 'processing' | 'agent_speaking'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  source?: 'voice' | 'text'
}

const WAKE_PEAK = 0.022
const SPEECH_PEAK = 0.025
const TTS_WATCHDOG_MS = 45000
const TEXT_TURN_ACK_MS = 6000
const MAX_UTTERANCE_MS = 25000

interface RuntimeParams {
  silenceMs: number
  bargeInPeak: number
  bargeInMs: number
  echoGuardMs: number
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
    endpointDebounceMs: 300,
    minEndpointChars: 3,
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
  let textSending = false
  let pendingTextTurn: string | null = null
  let textViaRest = false
  let turnAckWaiter: { resolve: (ok: boolean) => void; timer: ReturnType<typeof setTimeout> } | null =
    null
  let turnStartAt = 0
  let playbackMarked = false
  let lastEndpointAt = 0
  let lastTurnMetrics: import('@/services/realtimeSession').TurnMetrics | null = null

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
    messages.value.push({ role: 'user', content: trimmed, source })
  }

  function commitAssistantMessage(text: string) {
    const trimmed = text.trim()
    if (!trimmed) return
    const last = messages.value[messages.value.length - 1]
    if (last?.role === 'assistant' && last.content === trimmed) return
    messages.value.push({ role: 'assistant', content: trimmed })
  }

  function loadHistory(history: ChatMessage[]) {
    messages.value = history.map((m) => ({
      role: m.role,
      content: m.content,
      source: m.source,
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

  function finishTextTurn() {
    clearTtsWatchdog()
    clearTurnAckWait()
    replyText.value = ''
    partialText.value = ''
    textSending = false
    pendingTextTurn = null
    textViaRest = false
    if (recording) {
      ttsPlayer.markDone(() => {
        enterResting()
        usePetStore().syncAnimationFromState()
      })
    } else {
      setPhase('idle')
      statusText.value = '输入消息或开始语音对话'
      usePetStore().syncAnimationFromState()
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
          heardSpeech = true
          lastSpeechAt = Date.now()
          if (phase === 'resting') {
            setPhase('user_speaking')
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
  }

  function startSilenceWatch() {
    clearSilenceWatch()
    silenceTimer = setInterval(() => {
      if (!recording || phase !== 'user_speaking' || !heardSpeech || lastSpeechAt <= 0) return
      if (Date.now() - lastSpeechAt >= params.silenceMs) {
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
  function enterResting() {
    clearTtsWatchdog()
    setPhase('resting')
    submitLock = false
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    heardSpeech = false
    lastSpeechAt = 0
    utteranceStartedAt = 0
    bargeAccumMs = 0
    ttsStartedAt = 0
    resetTurnTiming()
    chunksSent.value = 0
    speechVad?.reset()
    if (recording) {
      statusText.value = 'Mochi 在休息... 说话我就听'
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

  /** Owner started speaking — wake up and begin uploading. */
  function wakeOnSpeech() {
    if (phase !== 'resting' || !recording) return
    setPhase('user_speaking')
    uploadSeq = 0
    chunksSentCount = 0
    peakSeen = 0
    heardSpeech = true
    lastSpeechAt = Date.now()
    utteranceStartedAt = Date.now()
    chunksSent.value = 0
    partialText.value = ''
    realtimeSession.sendAudioStart()
    statusText.value = '正在听...'
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

    if (chunksSentCount === 0) {
      enterResting()
      return
    }
    if (!force && peakSeen < 0.002) {
      enterResting()
      return
    }

    if (!realtimeSession.isOpen()) {
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }

    clearSilenceWatch()
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

    const sent = realtimeSession.sendAudioEnd()
    if (!sent) {
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
      wakeOnSpeech()
      return
    }

    if (phase !== 'user_speaking') return

    if (ev === 'speech_start') {
      heardSpeech = true
      lastSpeechAt = Date.now()
      statusText.value = '正在听...'
    }

    if (ev === 'speech_end' && heardSpeech) {
      void submitUtterance(false)
    }
  }

  function bargeIn() {
    if (phase !== 'agent_speaking') return
    ttsPlayer.stop()
    clearTtsWatchdog()
    replyText.value = ''
    realtimeSession.sendInterrupt()
    enterResting()
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
    speechVad = new HybridSpeechVad(handleVadEvent)
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
      })
      if (reply) {
        commitAssistantMessage(reply)
      } else {
        commitAssistantMessage('嗯... 让我想想~')
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

  async function connect() {
    if (connected.value && realtimeSession.isOpen()) return

    if (connected.value && !realtimeSession.isOpen()) {
      connected.value = false
      unsub?.()
      unsub = null
      realtimeSession.disconnect()
    }

    statusText.value = '连接中...'

    unsub = realtimeSession.on(handleEvent)
    try {
      await realtimeSession.connect()
    } catch {
      connected.value = false
      unsub?.()
      unsub = null
      statusText.value = '连接失败，请关闭面板重新打开'
      return
    }
    connected.value = true
    realtimeSession.sendPrewarm()
    statusText.value = recording ? '点击开始对话' : '输入消息或开始语音对话'
  }

  /** Keep /ws/voice open for push reminders (no mic). */
  async function ensurePushConnected() {
    if (connected.value) return
    try {
      await connect()
    } catch (e) {
      console.warn('[realtime] push connect skipped', e)
    }
  }

  function disconnect() {
    void endConversation()
    unsub?.()
    unsub = null
    realtimeSession.disconnect()
    connected.value = false
    statusText.value = ''
  }

  async function startCloudTalk() {
    await initVad()

    try {
      await capture.start((pcm, _seq) => {
        const boosted = amplifyPCM(pcm)
        const peak = pcmPeakLevel(boosted)
        micLevel.value = peak

        if (phase === 'resting') {
          speechVad?.feed(pcmToFloat(boosted))
          if (peak >= WAKE_PEAK) {
            wakeOnSpeech()
          }
          return
        }

        if (phase === 'user_speaking') {
          if (peak >= SPEECH_PEAK) {
            heardSpeech = true
            lastSpeechAt = Date.now()
          }
          speechVad?.feed(pcmToFloat(boosted))
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
        statusText.value = '麦克风权限被拒绝，请在浏览器设置中允许'
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

  async function startTalk() {
    if (recording) return

    await initClientConfig().catch(() => {})
    refreshRuntimeParams()

    await connect()
    realtimeSession.sendPrewarm()

    effectiveSttMode = resolveSttMode(getRealtimeConfig(), isLocalSttSupported())

    partialText.value = ''
    replyText.value = ''
    ttsPlayer.stop()
    clearTtsWatchdog()
    submitLock = false
    micLevel.value = 0

    if (effectiveSttMode === 'local') {
      await startLocalTalk()
      return
    }

    await startCloudTalk()
  }

  async function endConversation() {
    if (!recording) return
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
    await capture.stop()
    statusText.value = connected.value ? '点击开始对话' : ''
  }

  function handleEvent(ev: RealtimeEvent) {
    const pet = usePetStore()

    switch (ev.type) {
      case 'session_start':
        sessionId.value = ev.sessionId
        break
      case 'asr_partial':
        partialText.value = ev.text
        if (ev.sentenceEnd) {
          handleAsrEndpoint(ev.text)
        }
        break
      case 'asr_final':
        if (textSending) {
          partialText.value = ''
          break
        }
        commitUserMessage(ev.text, 'voice')
        partialText.value = ''
        replyText.value = ''
        setPhase('processing')
        startTtsWatchdog()
        statusText.value = 'Mochi 正在想...'
        break
      case 'llm_token':
        if (textViaRest) break
        if (recording && phase === 'processing') {
          statusText.value = 'Mochi 正在回复...'
        }
        replyText.value += ev.token
        break
      case 'llm_done':
        if (textViaRest) break
        replyText.value = ev.text
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
      case 'tts_audio':
        setPhase('agent_speaking')
        if (!ttsStartedAt) ttsStartedAt = Date.now()
        statusText.value = 'Mochi 正在说话...（大声说话可打断）'
        ttsPlayer.enqueue(ev.pcm, ev.format, markPlaybackStart)
        break
      case 'tts_done':
        if (phase !== 'resting' && phase !== 'idle') {
          finishTextTurn()
        } else {
          clearTtsWatchdog()
          textSending = false
        }
        resetTurnTiming()
        break
      case 'turn_metrics':
        lastTurnMetrics = ev.metrics
        if (import.meta.env.DEV) {
          console.debug('[realtime] turn_metrics', ev.metrics)
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
        enterResting()
        pet.setAnimation('happy')
        break
      case 'animation':
        if (ev.state === 'listening') pet.setAnimation('happy')
        else if (ev.state === 'thinking') pet.setAnimation('idle')
        else if (ev.state === 'speaking') pet.setAnimation('happy')
        else if (ev.state === 'idle') pet.setAnimation('idle')
        else pet.syncAnimationFromState()
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
        handleProactiveMessage({ message: ev.message, animation: ev.animation })
        commitAssistantMessage(ev.message)
        break
      case 'disconnected':
        connected.value = false
        talking.value = false
        recording = false
        clearSilenceWatch()
        clearTurnAckWait()
        if (textSending && pendingTextTurn) {
          clearTtsWatchdog()
          void sendTextViaRest(pendingTextTurn)
          statusText.value = '连接断开，改用文字通道...'
          void connect().catch(() => {})
          break
        }
        clearTtsWatchdog()
        textSending = false
        pendingTextTurn = null
        setPhase('idle')
        resting.value = false
        statusText.value = '连接断开，正在重连...'
        void connect().catch(() => {
          statusText.value = '连接断开，请关闭面板重新打开'
        })
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
    ensurePushConnected,
    disconnect,
    startTalk,
    sendTextMessage,
    loadHistory,
    appendAssistantMessage: commitAssistantMessage,
    submitUtterance,
    endConversation,
    stopTalk: submitUtterance,
  }
})
