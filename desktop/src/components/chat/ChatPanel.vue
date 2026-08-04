<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { usePetStore } from '@/stores/petStore'
import { useRealtimeStore, type ChatMessage } from '@/stores/realtimeStore'
import { closeChatPanel, isTauri } from '@/services/chatWindow'
import { getChatHistory } from '@/services/api'
import { getClientConfig, initClientConfig } from '@/config'
import { listenProactive } from '@/services/proactiveSync'
import {
  claimVoiceOwner,
  getStoredVoiceOwner,
  releaseVoiceOwner,
  setupVoiceOwnerListener,
} from '@/services/voiceSessionOwner'
import { warmUpMicrophoneAccess } from '@/utils/micPermission'
import { useAuthStore } from '@/stores/authStore'

const props = defineProps<{ floating?: boolean; compact?: boolean; docked?: boolean }>()

type HistoryRow = { role: string; content: string; created_at?: string }

type DisplayItem =
  | { kind: 'divider'; label: string; key: string }
  | { kind: 'message'; message: ChatMessage; index: number; key: string }

function parseDate(ts?: string | number): Date | null {
  if (ts == null || ts === '') return null
  const d = typeof ts === 'number' ? new Date(ts) : new Date(ts)
  return Number.isNaN(d.getTime()) ? null : d
}

function dayKey(ts?: string | number): string {
  const d = parseDate(ts)
  return d ? d.toDateString() : ''
}

function formatDayLabel(ts?: string | number): string {
  const d = parseDate(ts)
  if (!d) return ''
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return '今天'
  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (d.toDateString() === yesterday.toDateString()) return '昨天'
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

function formatMessageTime(ts?: string | number): string {
  const d = parseDate(ts)
  if (!d) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 系统/拒答/未回应 — 灰色居中条，不进对话气泡流 */
function isSystemMessage(m: ChatMessage): boolean {
  if (m.dismissed) return true
  if (m.role === 'assistant' && /不是主人|只能听主人/.test(m.content)) return true
  return false
}

const pet = usePetStore()
const rt = useRealtimeStore()
const textInput = ref('')
const scrollEl = ref<HTMLElement | null>(null)
const realtimeEnabled = ref(true)
const auth = useAuthStore()
let unlistenProactive: (() => void) | null = null
let unlistenChatOpened: (() => void) | null = null
let unlistenSidePanelOpened: (() => void) | null = null
let unlistenVoiceOwner: (() => void) | null = null

const showStreamingReply = computed(() => {
  if (!rt.replyText || !rt.processing) return false
  const last = rt.messages[rt.messages.length - 1]
  return !(last?.role === 'assistant' && last.content === rt.replyText)
})

const displayItems = computed((): DisplayItem[] => {
  const items: DisplayItem[] = []
  let lastDay = ''
  rt.messages.forEach((m, index) => {
    const dk = dayKey(m.createdAt)
    if (dk && dk !== lastDay) {
      items.push({
        kind: 'divider',
        label: formatDayLabel(m.createdAt),
        key: `d-${dk}`,
      })
      lastDay = dk
    }
    items.push({ kind: 'message', message: m, index, key: `m-${index}` })
  })
  return items
})

const voiceStatus = computed(() => {
  if (!realtimeEnabled.value) return { text: '文字模式', tone: 'idle' as const }
  if (!rt.talking) return { text: '未开始', tone: 'idle' as const }
  if (rt.resting) return { text: '休息中', tone: 'resting' as const }
  if (rt.userSpeaking) return { text: '正在听', tone: 'listening' as const }
  if (rt.processing) return { text: '在想…', tone: 'thinking' as const }
  return { text: '在说…', tone: 'speaking' as const }
})

const voiceBtnMode = computed(() => {
  if (!realtimeEnabled.value) return 'off' as const
  if (!rt.talking) return 'start' as const
  if (rt.resting) return 'resting' as const
  if (rt.processing || (!rt.userSpeaking && !rt.resting)) return 'busy' as const
  return 'finish' as const
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

async function acquireChatVoice(options?: { autoStartTalk?: boolean }) {
  const autoStartTalk = options?.autoStartTalk ?? true
  auth.syncFromStorage()

  if (props.floating && isTauri()) {
    rt.setVoiceWindow('chat')
    await claimVoiceOwner('chat')
    if (getStoredVoiceOwner() !== 'chat') {
      await claimVoiceOwner('chat')
    }
    await new Promise((resolve) => setTimeout(resolve, 80))
    await rt.connectIfOwner()
  } else {
    rt.setVoiceWindow('inline')
    await rt.connectIfOwner()
  }

  if (autoStartTalk && realtimeEnabled.value && !rt.talking) {
    await rt.startTalk().catch(() => {})
  }
}

async function onStartTalk() {
  await acquireChatVoice({ autoStartTalk: false })
  const ok = await rt.startTalk()
  if (!ok) {
    rt.statusText = rt.statusText || '无法启动语音，请检查 X-ASR 服务'
    return
  }
  // 与桌宠单击一致：点话筒即进入「正在听」
  if (rt.resting) {
    const wake = await rt.wakeListening({ manual: true })
    if (!wake.ok && wake.reason === 'voiceprint_missing') {
      rt.statusText = '请先在设置中录入主人声纹'
    }
  }
}

async function releaseChatVoice() {
  if (props.floating && isTauri()) {
    await rt.yieldVoiceConnection()
    await releaseVoiceOwner('chat')
  }
}

async function close() {
  pet.isChatOpen = false
  await releaseChatVoice()
  if (isTauri()) {
    await closeChatPanel()
    try {
      const { emit } = await import('@tauri-apps/api/event')
      await emit('chat-closed', {})
      await emit('side-panel-closed', { mode: 'chat' })
    } catch {
      // optional
    }
  }
}

async function finishSpeaking() {
  rt.submitUtterance(true)
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

function onVoiceBtnClick() {
  if (voiceBtnMode.value === 'start') void onStartTalk()
  else if (voiceBtnMode.value === 'finish') void finishSpeaking()
  else if (voiceBtnMode.value === 'resting') void rt.wakeListening({ manual: true })
}

onMounted(async () => {
  await initClientConfig().catch(() => {})
  realtimeEnabled.value = getClientConfig().realtimeEnabled

  unlistenProactive = await listenProactive((payload) => {
    rt.appendAssistantMessage(payload.message)
  })

  try {
    const history = (await getChatHistory()) as HistoryRow[] | null
    if (Array.isArray(history) && history.length > 0) {
      rt.loadHistory(
        history.map((m) => ({
          role: m.role as 'user' | 'assistant',
          content: m.content,
          createdAt: m.created_at,
        })),
      )
    }
  } catch {
    // history optional
  }

  if (props.floating && isTauri()) {
    void warmUpMicrophoneAccess()
    unlistenVoiceOwner = await setupVoiceOwnerListener('chat', {
      onAcquire: () => rt.connectIfOwner(),
      onYield: () => rt.yieldVoiceConnection(),
    })
    try {
      const { listen } = await import('@tauri-apps/api/event')
      const onChatPanelShow = () => {
        void acquireChatVoice().catch(() => {
          rt.statusText = realtimeEnabled.value ? '连接失败' : '文字模式'
        })
      }
      // 仅聊天模式激活语音；设置模式也会 emit side-panel-opened，不能误触
      const onSidePanelOpened = (event: { payload?: { mode?: string } }) => {
        const mode = event.payload?.mode
        if (mode === 'settings') return
        if (mode === 'chat' || !mode) onChatPanelShow()
      }
      unlistenSidePanelOpened = await listen('side-panel-opened', onSidePanelOpened)
      unlistenChatOpened = await listen('chat-opened', onSidePanelOpened)
    } catch {
      // optional
    }
  } else {
    await acquireChatVoice().catch(() => {
      rt.statusText = realtimeEnabled.value ? '连接失败' : '文字模式'
    })
  }
})

onUnmounted(() => {
  void releaseChatVoice()
  unlistenVoiceOwner?.()
  unlistenVoiceOwner = null
  unlistenProactive?.()
  unlistenProactive = null
  unlistenChatOpened?.()
  unlistenChatOpened = null
  unlistenSidePanelOpened?.()
  unlistenSidePanelOpened = null
})
</script>

<template>
  <div class="chat-root" :class="{ floating, compact, docked }">
    <div class="chat-panel">
      <header class="chat-header">
        <div class="header-left">
          <span class="pet-name">{{ pet.petName }}</span>
          <span class="status-pill" :class="'tone-' + voiceStatus.tone">
            <span class="status-dot" />
            {{ voiceStatus.text }}
          </span>
        </div>
        <div class="header-actions">
          <button
            v-if="rt.talking && realtimeEnabled"
            type="button"
            class="header-link"
            @click="stopConversation"
          >
            结束
          </button>
          <button class="close-btn" type="button" aria-label="关闭" @click="close">✕</button>
        </div>
      </header>

      <div v-if="rt.talking && realtimeEnabled" class="mic-meter-wrap">
        <div class="mic-meter">
          <div class="mic-meter-bar" :style="{ width: Math.round(rt.micLevel * 100) + '%' }" />
        </div>
      </div>

      <div ref="scrollEl" class="chat-messages">
        <div v-if="rt.messages.length === 0 && !rt.partialText && !showStreamingReply" class="empty-hint">
          <p class="empty-title">和 {{ pet.petName }} 说点什么吧~</p>
          <p class="empty-sub">打字发送，或点 🎤 开始说话</p>
        </div>

        <template v-for="item in displayItems" :key="item.key">
          <div v-if="item.kind === 'divider'" class="date-divider">
            <span>{{ item.label }}</span>
          </div>
          <div
            v-else-if="isSystemMessage(item.message)"
            class="system-line"
          >
            {{ item.message.content }}
            <span v-if="item.message.dismissed" class="system-sub">已听到 · 未回应</span>
          </div>
          <div
            v-else
            class="message"
            :class="item.message.role"
          >
            <div class="message-col">
              <div class="bubble">{{ item.message.content }}</div>
              <span v-if="item.message.createdAt" class="msg-time">{{
                formatMessageTime(item.message.createdAt)
              }}</span>
            </div>
          </div>
        </template>

        <div v-if="rt.partialText" class="message user">
          <div class="bubble streaming"><span class="streaming-tag">识别中</span>{{ rt.partialText }}</div>
        </div>
        <div v-if="showStreamingReply" class="message assistant">
          <div class="bubble streaming">{{ rt.replyText }}</div>
        </div>
        <div v-if="rt.userSpeaking && !rt.partialText" class="message user">
          <div class="bubble streaming hint-bubble">正在听，请说话…</div>
        </div>
      </div>

      <footer class="chat-composer">
        <p v-if="!realtimeEnabled" class="realtime-hint">当前未开启实时语音，请使用文字聊天</p>
        <div v-else class="composer-row">
          <button
            type="button"
            class="voice-btn"
            :class="voiceBtnMode"
            :disabled="voiceBtnMode === 'busy' || voiceBtnMode === 'off'"
            :title="
              voiceBtnMode === 'start'
                ? '开始对话'
                : voiceBtnMode === 'finish'
                  ? '说完了'
                  : voiceBtnMode === 'resting'
                    ? '休息中，说话即可'
                    : ''
            "
            @click="onVoiceBtnClick"
          >
            🎤
          </button>
          <input
            v-model="textInput"
            type="text"
            class="text-field"
            placeholder="输入消息…"
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
      </footer>
    </div>
  </div>
</template>

<style scoped>
.chat-root {
  position: relative;
  width: 100%;
  height: 100%;
  background: var(--mochi-surface, #fff);
  border-radius: var(--mochi-radius-lg, 16px);
  overflow: hidden;
  box-shadow: var(--mochi-shadow, 0 8px 32px rgba(0, 0, 0, 0.18));
}

.chat-root.docked {
  width: 320px;
  height: 440px;
  flex-shrink: 0;
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
  gap: 8px;
  padding: 10px 12px;
  background: linear-gradient(135deg, var(--mochi-pink, #ff8fab), var(--mochi-pink-soft, #ffb3c6));
  color: white;
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.pet-name {
  font-weight: 600;
  font-size: 14px;
  white-space: nowrap;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.22);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 120px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.9);
  flex-shrink: 0;
}

.tone-listening .status-dot,
.tone-speaking .status-dot {
  animation: pulse-dot 1.2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.header-link {
  background: none;
  border: none;
  color: white;
  font-size: 12px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
  opacity: 0.95;
}

.header-link:hover {
  background: rgba(255, 255, 255, 0.2);
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

.mic-meter-wrap {
  padding: 0 12px 4px;
  background: linear-gradient(180deg, var(--mochi-pink-soft, #ffb3c6) 0%, #fff 100%);
}

.mic-meter {
  height: 3px;
  background: rgba(255, 255, 255, 0.5);
  border-radius: 2px;
  overflow: hidden;
}

.mic-meter-bar {
  height: 100%;
  background: linear-gradient(90deg, #7bed9f, #ff6b8a);
  transition: width 0.05s linear;
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--mochi-bg, #fafafa);
}

.empty-hint {
  text-align: center;
  margin-top: 28px;
}

.empty-title {
  margin: 0;
  font-size: 14px;
  color: #666;
}

.empty-sub {
  margin: 6px 0 0;
  font-size: 12px;
  color: #aaa;
}

.date-divider {
  display: flex;
  justify-content: center;
  margin: 4px 0;
}

.date-divider span {
  font-size: 11px;
  color: #aaa;
  padding: 2px 10px;
  background: #eee;
  border-radius: 999px;
}

.system-line {
  text-align: center;
  font-size: 12px;
  color: #999;
  padding: 6px 12px;
  margin: 2px 16px;
  background: #ececec;
  border-radius: 8px;
  line-height: 1.45;
}

.system-sub {
  display: block;
  font-size: 10px;
  margin-top: 2px;
  opacity: 0.85;
}

.message {
  display: flex;
}

.message-col {
  display: flex;
  flex-direction: column;
  max-width: 85%;
  gap: 2px;
}

.message.user {
  justify-content: flex-end;
}

.message.user .message-col {
  align-items: flex-end;
}

.message.assistant {
  justify-content: flex-start;
}

.message.assistant .message-col {
  align-items: flex-start;
}

.msg-time {
  font-size: 10px;
  color: #aaa;
  padding: 0 4px;
}

.bubble {
  padding: 8px 12px;
  border-radius: 14px;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.message.user .bubble {
  background: var(--mochi-pink, #ff8fab);
  color: white;
  border-bottom-right-radius: 4px;
}

.message.assistant .bubble {
  background: var(--mochi-surface, #fff);
  color: var(--mochi-text, #333);
  border: 1px solid var(--mochi-border, #f0f0f0);
  border-bottom-left-radius: 4px;
}

.bubble.streaming {
  opacity: 0.92;
}

.bubble.hint-bubble {
  background: #e8e8e8;
  color: #666;
}

.streaming-tag {
  font-size: 11px;
  font-weight: 600;
  margin-right: 4px;
  opacity: 0.9;
}

.chat-composer {
  flex-shrink: 0;
  padding: 8px 12px 12px;
  background: var(--mochi-surface, #fff);
  border-top: 1px solid var(--mochi-border, #f0f0f0);
}

.realtime-hint {
  margin: 0;
  font-size: 12px;
  color: #888;
  text-align: center;
}

.composer-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.voice-btn {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border: none;
  border-radius: 50%;
  background: var(--mochi-pink-bg, #fff0f3);
  font-size: 16px;
  cursor: pointer;
  line-height: 1;
}

.voice-btn.start {
  background: linear-gradient(135deg, var(--mochi-pink, #ff8fab), var(--mochi-pink-soft, #ffb3c6));
}

.voice-btn.resting {
  box-shadow: 0 0 0 2px var(--mochi-pink, #ff8fab);
}

.voice-btn:disabled {
  opacity: 0.45;
  cursor: default;
}

.text-field {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--mochi-border, #eee);
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 13px;
  outline: none;
}

.text-field:focus {
  border-color: var(--mochi-pink-soft, #ffb3c6);
}

.send-btn {
  flex-shrink: 0;
  border: none;
  border-radius: 10px;
  padding: 8px 12px;
  background: var(--mochi-pink, #ff8fab);
  color: white;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

/* compact / floating 缩放 */
.chat-root.compact .pet-name {
  font-size: 11px;
  max-width: 48px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chat-root.compact .status-pill {
  font-size: 9px;
  max-width: 72px;
  padding: 1px 6px;
}

.chat-root.compact .chat-messages {
  padding: 6px 8px;
}

.chat-root.compact .bubble {
  font-size: 11px;
  padding: 6px 8px;
}

.chat-root.compact .composer-row {
  gap: 4px;
}

.chat-root.compact .voice-btn {
  width: 28px;
  height: 28px;
  font-size: 13px;
}

.chat-root.compact .text-field,
.chat-root.compact .send-btn {
  font-size: 10px;
  padding: 5px 8px;
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

.chat-root.floating.panel-on-left::after {
  left: auto;
  right: 22px;
}
</style>
