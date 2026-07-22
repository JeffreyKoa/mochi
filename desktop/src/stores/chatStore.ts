import { defineStore } from 'pinia'
import { ref } from 'vue'
import { voiceChat } from '@/services/voiceApi'
import { playBase64Audio, VoiceRecorder } from '@/services/voice'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export const useChatStore = defineStore('chat', () => {
  const messages = ref<ChatMessage[]>([])
  const isProcessing = ref(false)
  const isRecording = ref(false)
  const statusText = ref('')
  const recorder = new VoiceRecorder()

  function addMessage(role: 'user' | 'assistant', content: string) {
    messages.value.push({ role, content })
  }

  async function startRecording() {
    if (isProcessing.value || isRecording.value) return
    try {
      await recorder.start()
      isRecording.value = true
      statusText.value = '正在听...'
    } catch {
      statusText.value = '无法访问麦克风'
    }
  }

  async function stopRecordingAndSend() {
    if (!isRecording.value) return
    isRecording.value = false
    isProcessing.value = true
    statusText.value = '识别中...'

    try {
      const blob = await recorder.stop()
      if (blob.size < 1000) {
        statusText.value = '录音太短，请重试'
        return
      }

      const result = await voiceChat(blob)
      addMessage('user', result.transcript)
      addMessage('assistant', result.reply)

      if (result.audio) {
        statusText.value = '播放回复...'
        await playBase64Audio(result.audio, result.format || 'mp3')
      } else if (result.tts_error) {
        statusText.value = '语音合成失败，已显示文字'
      }
      statusText.value = '按住说话'
    } catch (e) {
      statusText.value = e instanceof Error ? e.message : '发送失败'
    } finally {
      isProcessing.value = false
      if (statusText.value !== '按住说话' && !statusText.value.includes('失败')) {
        setTimeout(() => {
          if (!isRecording.value && !isProcessing.value) {
            statusText.value = '按住说话'
          }
        }, 2000)
      }
    }
  }

  function cancelRecording() {
    if (isRecording.value) {
      isRecording.value = false
      recorder.stop().catch(() => {})
      statusText.value = '按住说话'
    }
  }

  function setHistory(history: ChatMessage[]) {
    messages.value = history.map((m) => ({
      role: m.role,
      content: m.content,
    }))
    statusText.value = '按住说话'
  }

  return {
    messages,
    isProcessing,
    isRecording,
    statusText,
    addMessage,
    startRecording,
    stopRecordingAndSend,
    cancelRecording,
    setHistory,
  }
})
