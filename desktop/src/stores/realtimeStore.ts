import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { PCMCapture, arrayBufferToBase64, pcmPeakLevel, amplifyPCM } from '@/services/pcmCapture'
import { realtimeSession, type RealtimeEvent } from '@/services/realtimeSession'
import { usePetStore } from '@/stores/petStore'
import { TTSAudioQueue } from '@/services/ttsAudioPlayer'
import { HybridSpeechVad, pcmToFloat, type VADEvent } from '@/services/sileroSpeechVad'

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
const SILENCE_MS = 900
const BARGE_IN_PEAK = 0.09
const BARGE_IN_MS = 800
const ECHO_GUARD_MS = 3200
const TTS_WATCHDOG_MS = 45000
const MAX_UTTERANCE_MS = 25000

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

  function finishTextTurn() {
    clearTtsWatchdog()
    replyText.value = ''
    partialText.value = ''
    textSending = false
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

  function startSilenceWatch() {
    clearSilenceWatch()
    silenceTimer = setInterval(() => {
      if (!recording || phase !== 'user_speaking' || !heardSpeech || lastSpeechAt <= 0) return
      if (Date.now() - lastSpeechAt >= SILENCE_MS) {
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
    chunksSent.value = 0
    speechVad?.reset()
    if (recording) {
      statusText.value = 'Mochi 在休息... 说话我就听'
      usePetStore().setAnimation('idle')
      startSilenceWatch()
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
    if (Date.now() - ttsStartedAt < ECHO_GUARD_MS) return

    if (peak >= BARGE_IN_PEAK) {
      bargeAccumMs += 20
      if (bargeAccumMs >= BARGE_IN_MS) {
        bargeIn()
      }
    } else {
      bargeAccumMs = 0
    }
  }

  function startTtsWatchdog() {
    clearTtsWatchdog()
    ttsWatchdog = setTimeout(() => {
      if (phase === 'agent_speaking' || phase === 'processing') {
        ttsPlayer.stop()
        enterResting()
        statusText.value = '语音超时，请继续说话'
      }
    }, TTS_WATCHDOG_MS)
  }

  async function initVad() {
    speechVad?.destroy()
    speechVad = new HybridSpeechVad(handleVadEvent)
    await speechVad.init()
  }

  async function sendTextMessage(text: string) {
    const trimmed = text.trim()
    if (!trimmed || textSending) return

    await connect()
    if (!realtimeSession.isOpen()) {
      statusText.value = '连接断开，请关闭面板重新打开'
      return
    }
    if (processingRef.value && !recording) {
      statusText.value = 'Mochi 正在回复，请稍候...'
      return
    }

    textSending = true
    commitUserMessage(trimmed, 'text')
    partialText.value = ''
    replyText.value = ''
    setPhase('processing')
    startTtsWatchdog()
    statusText.value = 'Mochi 正在想...'

    const sent = realtimeSession.sendTextInput(trimmed)
    if (!sent) {
      textSending = false
      messages.value.pop()
      statusText.value = '发送失败，请重试'
      setPhase('idle')
    }
  }

  async function connect() {
    if (connected.value) return
    statusText.value = '连接中...'

    unsub = realtimeSession.on(handleEvent)
    await realtimeSession.connect()
    connected.value = true
    statusText.value = recording ? '点击开始对话' : '输入消息或开始语音对话'
  }

  function disconnect() {
    void endConversation()
    unsub?.()
    unsub = null
    realtimeSession.disconnect()
    connected.value = false
    statusText.value = ''
  }

  async function startTalk() {
    if (recording) return

    await connect()
    await initVad()

    partialText.value = ''
    replyText.value = ''
    ttsPlayer.stop()
    clearTtsWatchdog()
    submitLock = false
    micLevel.value = 0

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
        break
      case 'asr_final':
        commitUserMessage(ev.text, 'voice')
        partialText.value = ''
        replyText.value = ''
        setPhase('processing')
        startTtsWatchdog()
        statusText.value = 'Mochi 正在想...'
        break
      case 'llm_token':
        if (recording && phase === 'processing') {
          statusText.value = 'Mochi 正在回复...'
        }
        replyText.value += ev.token
        break
      case 'llm_done':
        replyText.value = ev.text
        commitAssistantMessage(ev.text)
        if (recording) {
          if (phase !== 'agent_speaking') {
            statusText.value = 'Mochi 正在回复...'
          }
        } else {
          statusText.value = 'Mochi 已回复'
        }
        break
      case 'tts_audio':
        setPhase('agent_speaking')
        if (!ttsStartedAt) ttsStartedAt = Date.now()
        statusText.value = 'Mochi 正在说话...（大声说话可打断）'
        ttsPlayer.enqueue(ev.pcm, ev.format)
        break
      case 'tts_done':
        finishTextTurn()
        break
      case 'turn_ack':
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
        if (ev.code !== 'ASR_FAILED') {
          replyText.value = ev.message
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
      case 'disconnected':
        connected.value = false
        talking.value = false
        recording = false
        clearSilenceWatch()
        clearTtsWatchdog()
        setPhase('idle')
        resting.value = false
        statusText.value = '连接断开'
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
    disconnect,
    startTalk,
    sendTextMessage,
    loadHistory,
    submitUtterance,
    endConversation,
    stopTalk: submitUtterance,
  }
})
