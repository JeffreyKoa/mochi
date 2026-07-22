<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { usePetStore } from '@/stores/petStore'
import { useRealtimeStore } from '@/stores/realtimeStore'
import { closeChatPanel, isTauri } from '@/services/chatWindow'
import { getChatHistory } from '@/services/api'

defineProps<{ floating?: boolean; compact?: boolean }>()

const pet = usePetStore()
const rt = useRealtimeStore()
const textInput = ref('')
const scrollEl = ref<HTMLElement | null>(null)

const showStreamingReply = computed(() => {
  if (!rt.replyText || !rt.processing) return false
  const last = rt.messages[rt.messages.length - 1]
  return !(last?.role === 'assistant' && last.content === rt.replyText)
})

async function scrollToBottom() {
  await nextTick()
  const el = scrollEl.value
  if (el) el.scrollTop = el.scrollHeight
}

watch(
  () => [rt.messages.length, rt.partialText, rt.replyText, rt.processing] as const,
  () => void scrollToBottom(),
)

async function close() {
  rt.disconnect()
  pet.isChatOpen = false
  if (isTauri()) {
    await closeChatPanel()
    try {
      await import('@tauri-apps/api/event').then(({ emit }) => emit('chat-closed', {}))
    } catch {
      // optional
    }
  }
}

async function finishSpeaking() {
  rt.submitUtterance(false)
}

async function stopConversation() {
  await rt.endConversation()
}

async function sendText() {
  const text = textInput.value.trim()
  if (!text) return
  textInput.value = ''
  await rt.sendTextMessage(text)
}

onMounted(async () => {
  try {
    const history = await getChatHistory()
    if (Array.isArray(history) && history.length > 0) {
      rt.loadHistory(
        history.map((m: { role: string; content: string }) => ({
          role: m.role as 'user' | 'assistant',
          content: m.content,
        })),
      )
    }
  } catch {
    // history optional
  }

  rt.connect().catch(() => {
    rt.statusText = '连接失败'
  })
})

onUnmounted(() => {
  rt.disconnect()
})
</script>

<template>
  <div class="chat-root" :class="{ floating, compact }">
    <div class="chat-panel">
      <div class="chat-header">
        <span>{{ pet.petName }}</span>
        <button class="close-btn" type="button" aria-label="关闭" @click="close">✕</button>
      </div>

      <div ref="scrollEl" class="chat-messages">
        <div v-if="rt.messages.length === 0 && !rt.partialText && !showStreamingReply" class="empty-hint">
          打字或点「开始」说话<br />说话 → 文字+语音；打字 → 仅文字
        </div>
        <div
          v-for="(m, i) in rt.messages"
          :key="i"
          class="message"
          :class="m.role"
        >
          <div class="bubble">{{ m.content }}</div>
        </div>
        <div v-if="rt.partialText" class="message user">
          <div class="bubble streaming">{{ rt.partialText }}</div>
        </div>
        <div v-if="showStreamingReply" class="message assistant">
          <div class="bubble streaming">{{ rt.replyText }}</div>
        </div>
        <div v-if="rt.userSpeaking && !rt.partialText" class="message user">
          <div class="bubble streaming">正在听...</div>
        </div>
      </div>

      <div class="text-input-row">
        <input
          v-model="textInput"
          type="text"
          class="text-field"
          placeholder="输入消息..."
          :disabled="rt.processing && !rt.talking"
          @keydown.enter.prevent="sendText"
        />
        <button
          class="send-btn"
          type="button"
          :disabled="!textInput.trim() || (rt.processing && !rt.talking)"
          @click="sendText"
        >
          发送
        </button>
      </div>

      <div class="voice-area">
        <p class="status">{{ rt.statusText }}</p>
        <div v-if="rt.talking" class="mic-meter">
          <div class="mic-meter-bar" :style="{ width: Math.round(rt.micLevel * 100) + '%' }" />
        </div>
        <button v-if="!rt.talking" class="mic-btn" type="button" @click="rt.startTalk()">
          开始对话
        </button>
        <button v-else-if="rt.resting" class="mic-btn resting" type="button" disabled>
          休息中 · 说话即可
        </button>
        <button v-else-if="rt.processing" class="mic-btn resting" type="button" disabled>
          处理中...
        </button>
        <button v-else class="mic-btn recording" type="button" @click="finishSpeaking">
          说完了
        </button>
        <button v-if="rt.talking" class="end-link" type="button" @click="stopConversation">
          结束
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-root {
  position: relative;
  width: 100%;
  height: 100%;
  background: #fff;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.18);
}

.chat-panel {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-weight: 600;
  font-size: 14px;
}

.close-btn {
  background: none;
  border: none;
  color: white;
  font-size: 15px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
  line-height: 1;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.2);
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.empty-hint {
  text-align: center;
  color: #aaa;
  font-size: 12px;
  margin-top: 32px;
  line-height: 1.6;
}

.message {
  display: flex;
}

.message.user {
  justify-content: flex-end;
}

.message.assistant {
  justify-content: flex-start;
}

.bubble {
  max-width: 85%;
  padding: 8px 12px;
  border-radius: 14px;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
}

.message.user .bubble {
  background: #ff8fab;
  color: white;
  border-bottom-right-radius: 4px;
}

.message.assistant .bubble {
  background: #f3f3f3;
  color: #333;
  border-bottom-left-radius: 4px;
}

.bubble.streaming {
  opacity: 0.7;
}

.text-input-row {
  display: flex;
  gap: 8px;
  padding: 8px 14px 0;
  align-items: center;
}

.text-field {
  flex: 1;
  border: 1px solid #eee;
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 13px;
  outline: none;
}

.text-field:focus {
  border-color: #ffb3c6;
}

.text-field:disabled {
  background: #fafafa;
  color: #aaa;
}

.send-btn {
  border: none;
  border-radius: 10px;
  padding: 8px 12px;
  background: #ff8fab;
  color: white;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.voice-area {
  padding: 12px 14px 14px;
  border-top: 1px solid #f0f0f0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.status {
  font-size: 12px;
  color: #888;
  min-height: 16px;
  text-align: center;
}

.mic-meter {
  width: 100%;
  height: 4px;
  background: #eee;
  border-radius: 2px;
  overflow: hidden;
}

.mic-meter-bar {
  height: 100%;
  background: linear-gradient(90deg, #7bed9f, #ff6b8a);
  transition: width 0.05s linear;
}

.mic-btn {
  width: 100%;
  padding: 10px;
  border: none;
  border-radius: 12px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}

.mic-btn.resting {
  background: #dfe6e9;
  color: #636e72;
  cursor: default;
}

.mic-btn.recording {
  background: linear-gradient(135deg, #ff6b8a, #ff8fab);
}

.end-link {
  background: none;
  border: none;
  color: #aaa;
  font-size: 11px;
  cursor: pointer;
  text-decoration: underline;
  padding: 0;
}

.chat-root.floating {
  overflow: visible;
  border-radius: 14px;
}

.chat-root.floating .chat-panel {
  border-radius: 14px;
  overflow: hidden;
}

.chat-root.floating::after {
  content: '';
  position: absolute;
  left: 22px;
  bottom: -9px;
  width: 16px;
  height: 16px;
  background: #fff;
  transform: rotate(45deg);
  box-shadow: 3px 3px 6px rgba(0, 0, 0, 0.08);
  z-index: -1;
}

.chat-root.floating.compact::after {
  left: 18px;
  right: auto;
  bottom: -7px;
  width: 12px;
  height: 12px;
}

.chat-root.floating .chat-header {
  padding: 10px 12px;
  font-size: 13px;
  border-radius: 14px 14px 0 0;
}

.chat-root.floating .chat-messages {
  padding: 10px;
}

.chat-root.floating .empty-hint {
  margin-top: 16px;
  font-size: 11px;
}

.chat-root.floating .voice-area {
  padding: 10px 12px 12px;
}

.chat-root.floating .mic-btn {
  padding: 9px;
  font-size: 12px;
}

.chat-root.compact .chat-header {
  padding: 5px 7px;
  font-size: 10px;
}

.chat-root.compact .chat-header span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 72px;
}

.chat-root.compact .close-btn {
  font-size: 12px;
  padding: 0 4px;
}

.chat-root.compact .chat-messages {
  padding: 4px;
}

.chat-root.compact .empty-hint {
  margin-top: 0;
  font-size: 9px;
  line-height: 1.35;
}

.chat-root.compact .text-input-row {
  padding: 4px 6px 0;
  gap: 4px;
}

.chat-root.compact .text-field {
  padding: 4px 6px;
  font-size: 10px;
}

.chat-root.compact .send-btn {
  padding: 4px 8px;
  font-size: 10px;
}

.chat-root.compact .voice-area {
  padding: 4px 6px 6px;
  gap: 3px;
}

.chat-root.compact .status {
  font-size: 9px;
  min-height: 10px;
}

.chat-root.compact .mic-btn {
  padding: 5px;
  font-size: 10px;
  border-radius: 8px;
}

.chat-root.compact .end-link {
  font-size: 9px;
}
</style>
