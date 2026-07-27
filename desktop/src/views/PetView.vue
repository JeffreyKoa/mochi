<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { PhysicalPosition } from '@tauri-apps/api/dpi'
import { useAuthStore } from '@/stores/authStore'
import { usePetStore } from '@/stores/petStore'
import { useGrowthStore } from '@/stores/growthStore'
import { useRealtimeStore } from '@/stores/realtimeStore'
import { interact, ApiError } from '@/services/api'
import { healthMonitor } from '@/services/healthMonitor'
import {
  PetRoamer,
  canRoam,
  restoreWindowPosition,
  saveWindowPosition,
} from '@/services/petRoaming'
import {
  ensurePetWindowVisible,
  closeChatPanel,
  hideSidePanelPopup,
  showSidePanelPopup,
  setPopupChatFollowsPet,
  isTauri,
} from '@/services/chatWindow'
import { getClientConfig, initClientConfig } from '@/config'
import PetCanvas from '@/components/pet/PetCanvas.vue'
import { onLipSync } from '@/services/voice'

const { sidePanelOpen = false } = defineProps<{ sidePanelOpen?: boolean }>()

const pet = usePetStore()
let unlistenLipSync: (() => void) | null = null
const auth = useAuthStore()
const growth = useGrowthStore()
const rt = useRealtimeStore()
const menuVisible = ref(false)
const menuPos = ref({ x: 0, y: 0 })
const menuEl = ref<HTMLElement | null>(null)
const bubbleEl = ref<HTMLElement | null>(null)
const bubbleStyle = ref<Record<string, string>>({})
const menuPosReady = ref(true)
const MENU_PAD = 6
const BUBBLE_PAD = 8
const isDragging = ref(false)
const dragMoved = ref(false)
const didDragWindow = ref(false)
const chatExternal = ref(false)
const lastHeadlessBubbleIndex = ref(0)

const DRAG_THRESHOLD = 5

let dragWindow: ReturnType<typeof getCurrentWindow> | null = null
let clickTimer: ReturnType<typeof setTimeout> | null = null
let suppressClick = false
let roamer: PetRoamer | null = null
let unlistenPetMoved: (() => void) | null = null

let dragPointerId = -1
let dragWindowBase = { x: 0, y: 0 }
let dragPointerStart = { x: 0, y: 0 }
let dragRaf = 0
let dragPendingPos: PhysicalPosition | null = null

async function interactWithRetry(type: 'touch' | 'feed' | 'play') {
  try {
    return await interact(type)
  } catch (e) {
    if (!(e instanceof ApiError) || (e.kind !== 'network' && e.kind !== 'server')) {
      throw e
    }
    const recovered = await healthMonitor.poke(() => {})
    if (recovered) {
      return await interact(type)
    }
    if (healthMonitor.watching) throw e
    return await new Promise<Awaited<ReturnType<typeof interact>>>((resolve, reject) => {
      pet.showPersistentBubble('网络有点卡，我在自动重连…')
      healthMonitor.start(async () => {
        pet.hideSpeechBubble()
        try {
          resolve(await interact(type))
        } catch (err) {
          reject(err)
        }
      })
    })
  }
}

function scheduleWindowMove(x: number, y: number) {
  if (!dragWindow) return
  dragPendingPos = new PhysicalPosition(Math.round(x), Math.round(y))
  if (dragRaf) return
  dragRaf = requestAnimationFrame(() => {
    dragRaf = 0
    const pos = dragPendingPos
    dragPendingPos = null
    if (pos && dragWindow) {
      void dragWindow.setPosition(pos)
    }
  })
}

function onWindowPointerMove(e: PointerEvent) {
  if (!isDragging.value || e.pointerId !== dragPointerId || !dragWindow) return
  const dx = e.screenX - dragPointerStart.x
  const dy = e.screenY - dragPointerStart.y
  if (!dragMoved.value && Math.hypot(dx, dy) < DRAG_THRESHOLD) return

  if (!dragMoved.value) {
    dragMoved.value = true
    didDragWindow.value = true
  }

  e.preventDefault()
  const dpr = window.devicePixelRatio || 1
  scheduleWindowMove(dragWindowBase.x + Math.round(dx * dpr), dragWindowBase.y + Math.round(dy * dpr))
}

async function onWindowPointerUp(e: PointerEvent) {
  if (!isDragging.value || e.pointerId !== dragPointerId) return

  window.removeEventListener('pointermove', onWindowPointerMove)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerUp)

  isDragging.value = false
  dragPointerId = -1

  if (dragRaf) {
    cancelAnimationFrame(dragRaf)
    dragRaf = 0
  }
  if (dragPendingPos && dragWindow) {
    await dragWindow.setPosition(dragPendingPos)
    dragPendingPos = null
  }

  if (dragMoved.value && dragWindow) {
    await saveWindowPosition(dragWindow)
  }

  setTimeout(() => {
    dragMoved.value = false
    didDragWindow.value = false
    if (!pet.isChatOpen && !rt.talking && !rt.processing) roamer?.resume()
  }, 50)
}

function onDragStart(e: PointerEvent) {
  if (e.button !== 0 || !dragWindow) return

  isDragging.value = true
  dragMoved.value = false
  didDragWindow.value = false
  dragPointerId = e.pointerId
  dragPointerStart = { x: e.screenX, y: e.screenY }
  roamer?.pause()

  void dragWindow.outerPosition().then((pos) => {
    dragWindowBase = { x: pos.x, y: pos.y }
  })

  window.addEventListener('pointermove', onWindowPointerMove)
  window.addEventListener('pointerup', onWindowPointerUp)
  window.addEventListener('pointercancel', onWindowPointerUp)
}

function startRoamer() {
  if (!dragWindow || roamer || sidePanelOpen) return
  roamer = new PetRoamer()
  roamer.start(dragWindow, {
    isPaused: () =>
      !canRoam(
        pet.lifeState.energy,
        pet.isChatOpen,
        isDragging.value,
        sidePanelOpen || growth.showSettings,
        rt.talking || rt.processing,
      ),
    onWalkStart: (facing) => {
      pet.setFacing(facing)
      pet.setRoaming(true)
      pet.setAnimation('walk')
    },
    onWalkEnd: () => {
      pet.setRoaming(false)
      pet.syncAnimationFromState()
    },
  })
}

watch(
  () => [pet.showBubble, pet.bubbleText, pet.facing] as const,
  () => {
    if (!pet.showBubble) {
      bubbleStyle.value = {}
      return
    }
    void nextTick(() => {
      layoutSpeechBubble()
      requestAnimationFrame(() => layoutSpeechBubble())
    })
  },
)

onMounted(async () => {
  window.addEventListener('resize', layoutSpeechBubble)
  unlistenLipSync = onLipSync((vol) => pet.setLipSyncVolume(vol))

  try {
    dragWindow = getCurrentWindow()
  } catch {
    dragWindow = null
  }

  if (dragWindow) {
    try {
      const restored = await restoreWindowPosition(dragWindow)
      if (!restored) await dragWindow.center()
    } catch {
      try {
        await dragWindow.center()
      } catch {
        // ignore
      }
    }

    try {
      await ensurePetWindowVisible()
    } catch {
      // ignore
    }

    startRoamer()
  }

  if (auth.isLoggedIn) {
    await initClientConfig().catch(() => {})
    rt.connect().catch(() => {})
  }

  try {
    const { listen } = await import('@tauri-apps/api/event')
    await listen('chat-closed', () => {
      pet.isChatOpen = false
      chatExternal.value = false
      pet.chatInline = false
      setPopupChatFollowsPet(false)
      if (!rt.talking && !rt.processing) roamer?.resume()
    })
    await listen('side-panel-closed', (event) => {
      const mode = (event.payload as { mode?: string } | undefined)?.mode
      if (mode === 'settings') growth.closeSettings()
      if (mode === 'chat' || !mode) {
        pet.isChatOpen = false
        pet.chatInline = false
        chatExternal.value = false
      }
      setPopupChatFollowsPet(false)
      if (!rt.talking && !rt.processing) roamer?.resume()
    })
    unlistenPetMoved = await listen('pet-window-moved', () => {
      // Handled natively by Rust WindowEvent::Moved
    })
  } catch {
    // optional
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', layoutSpeechBubble)
  unlistenLipSync?.()
  unlistenLipSync = null
  roamer?.stop()
  unlistenPetMoved?.()
  unlistenPetMoved = null
  window.removeEventListener('pointermove', onWindowPointerMove)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerUp)
  if (dragRaf) cancelAnimationFrame(dragRaf)
})

watch(
  () => [sidePanelOpen, growth.showSettings, pet.isChatOpen, rt.talking, rt.processing] as const,
  ([panelOpen, settings, chat, talking, processing]) => {
    if (panelOpen || settings || chat || talking || processing) {
      roamer?.pause()
      if (pet.isRoaming) {
        pet.setRoaming(false)
        pet.syncAnimationFromState()
      }
    } else {
      if (!roamer) startRoamer()
      else roamer.resume()
    }
  },
)

watch(
  () => pet.isChatOpen,
  async (open, wasOpen) => {
    if (open) {
      roamer?.pause()
      return
    }
    if (!wasOpen) return
    chatExternal.value = false
    pet.chatInline = false
    setPopupChatFollowsPet(false)
    const keepExpanded = growth.showSettings || sidePanelOpen
    await closeChatPanel(!keepExpanded).catch(() => {})
    if (!rt.talking && !rt.processing) roamer?.resume()
  },
)

watch(
  () => rt.messages.length,
  (len) => {
    if (pet.isChatOpen || len === 0 || pet.isReminderBubbleActive()) return
    if (rt.talking && pet.isVoiceBubbleActive()) return
    const last = rt.messages[len - 1]
    if (last?.role === 'assistant' && len > lastHeadlessBubbleIndex.value) {
      lastHeadlessBubbleIndex.value = len
      pet.showSpeechBubble(last.content, 12000)
    }
  },
)

watch(
  () => rt.replyText,
  (text) => {
    if (pet.isChatOpen || !rt.talking || pet.isReminderBubbleActive()) return
    const trimmed = text.trim()
    if (!trimmed) return
    if (pet.isVoiceBubbleActive()) {
      pet.updateVoiceBubble(trimmed)
    } else {
      pet.showVoiceBubble(trimmed)
    }
  },
)

watch(
  () => rt.partialText,
  (text) => {
    if (pet.isChatOpen || !rt.talking || !rt.userSpeaking || pet.isReminderBubbleActive()) return
    if (pet.isVoiceBubbleActive()) {
      pet.releaseVoiceBubble(0)
    }
    pet.showPersistentBubble(text.trim() || '正在听…')
  },
)

watch(
  () => rt.userSpeaking,
  (speaking) => {
    if (pet.isChatOpen || !rt.talking || pet.isReminderBubbleActive()) return
    if (speaking) {
      if (pet.isVoiceBubbleActive()) {
        pet.releaseVoiceBubble(0)
      }
      pet.showPersistentBubble(rt.partialText.trim() || '正在听…')
    }
  },
)

async function closeChatSurface(collapse = true) {
  pet.isChatOpen = false
  chatExternal.value = false
  pet.chatInline = false
  setPopupChatFollowsPet(false)
  if (isTauri()) {
    await closeChatPanel(collapse)
  }
}

async function openChat() {
  menuVisible.value = false

  if (pet.isChatOpen) {
    await closeChatSurface(true)
    if (!growth.showSettings && !sidePanelOpen) roamer?.resume()
    return
  }

  if (growth.showSettings) {
    growth.closeSettings()
    await hideSidePanelPopup()
  }

  roamer?.pause()
  auth.syncFromStorage()

  if (isTauri()) {
    const ok = await showSidePanelPopup('chat')
    if (!ok) {
      pet.showSpeechBubble('聊天打开失败，请重试~', 4000)
      roamer?.resume()
      return
    }
    chatExternal.value = true
    pet.chatInline = false
    pet.isChatOpen = true
    return
  }

  chatExternal.value = false
  pet.chatInline = true
  pet.isChatOpen = true
}

function openChatFromMenu() {
  closeMenu()
  void openChat()
}

async function startVoiceInteraction() {
  roamer?.pause()

  await initClientConfig().catch(() => {})
  if (!getClientConfig().realtimeEnabled) {
    pet.showSpeechBubble('语音对话未开启，请用聊天打字~')
    roamer?.resume()
    return false
  }

  pet.setAnimation('happy')
  pet.showSpeechBubble('我在听，主人说~', 2500)

  try {
    await rt.connect()
    const ok = await rt.startTalk()
    if (!ok) {
      const msg = rt.statusText || '无法启动语音，请稍后再试'
      pet.showSpeechBubble(msg, 6000)
      pet.syncAnimationFromState()
      roamer?.resume()
      return false
    }
    return true
  } catch {
    pet.showSpeechBubble('无法启动麦克风，请检查权限', 6000)
    pet.syncAnimationFromState()
    roamer?.resume()
    return false
  }
}

async function onPetClick() {
  if (didDragWindow.value || suppressClick) return
  if (pet.bootFailed) {
    pet.retryBoot()
    return
  }
  if (clickTimer) clearTimeout(clickTimer)
  clickTimer = setTimeout(async () => {
    clickTimer = null

    if (rt.talking) {
      if (rt.resting) {
        if (!rt.wakeListening()) {
          pet.showSpeechBubble('连接断开，正在重连…', 3000)
          void rt.connect().then(() => rt.startTalk())
        } else {
          pet.showSpeechBubble('我在听，主人说~', 2500)
        }
      }
      return
    }

    await startVoiceInteraction()
  }, 200)
}

async function startVoiceFromMenu() {
  closeMenu()
  await startVoiceInteraction()
}

async function endVoiceFromMenu() {
  closeMenu()
  await rt.endConversation()
  if (!rt.talking && !pet.isChatOpen) {
    pet.syncAnimationFromState()
    roamer?.resume()
  }
  pet.showSpeechBubble('好的，我先休息啦~', 2500)
}

async function onFeed() {
  menuVisible.value = false
  roamer?.pause()
  try {
    const result = await interactWithRetry('feed')
    pet.updateLifeState(result.state as Parameters<typeof pet.updateLifeState>[0])
    pet.setAnimation('eat')
    pet.showSpeechBubble('好吃~ 谢谢主人！')
    setTimeout(() => {
      pet.syncAnimationFromState()
      roamer?.resume()
    }, 3000)
  } catch {
    pet.showSpeechBubble('呜... 喂食失败了')
    roamer?.resume()
  }
}

async function onPlay() {
  menuVisible.value = false
  roamer?.pause()
  try {
    const result = await interactWithRetry('play')
    pet.updateLifeState(result.state as Parameters<typeof pet.updateLifeState>[0])
    pet.setAnimation('happy')
    pet.showSpeechBubble('好开心！')
    setTimeout(() => {
      pet.syncAnimationFromState()
      roamer?.resume()
    }, 2000)
  } catch {
    pet.showSpeechBubble('现在玩不动...')
    roamer?.resume()
  }
}

async function openSettingsFromMenu() {
  closeMenu()
  if (growth.showSettings) {
    growth.closeSettings()
    await hideSidePanelPopup()
    if (!pet.isChatOpen && !rt.talking && !rt.processing) roamer?.resume()
    return
  }

  if (pet.isChatOpen || chatExternal.value) {
    pet.isChatOpen = false
    pet.chatInline = false
    chatExternal.value = false
    setPopupChatFollowsPet(false)
    await hideSidePanelPopup()
  }

  if (isTauri()) {
    growth.openSettings()
    const ok = await showSidePanelPopup('settings')
    if (!ok) {
      growth.closeSettings()
      pet.showSpeechBubble('设置打开失败，请重试~', 4000)
    }
    return
  }

  growth.openSettings()
}

function layoutSpeechBubble() {
  const el = bubbleEl.value
  if (!el || !pet.showBubble) return

  const pad = 10
  bubbleStyle.value = {
    left: '50%',
    top: `${BUBBLE_PAD}px`,
    transform: 'translateX(-50%)',
    '--tail-x': '50%',
  }

  requestAnimationFrame(() => {
    const node = bubbleEl.value
    if (!node || !pet.showBubble) return

    const rect = node.getBoundingClientRect()
    let shift = 0
    if (rect.left < pad) shift += pad - rect.left
    if (rect.right > window.innerWidth - pad) {
      shift -= rect.right - (window.innerWidth - pad)
    }

    if (shift !== 0) {
      bubbleStyle.value = {
        left: '50%',
        top: `${BUBBLE_PAD}px`,
        transform: `translateX(calc(-50% + ${Math.round(shift)}px))`,
        '--tail-x': '50%',
      }
    }

    requestAnimationFrame(() => {
      const bubble = bubbleEl.value
      const area = bubble?.closest('.pet-area') as HTMLElement | null
      if (!bubble || !area) return
      const areaRect = area.getBoundingClientRect()
      const bubbleRect = bubble.getBoundingClientRect()
      const tailX = areaRect.left + areaRect.width / 2 - bubbleRect.left
      bubbleStyle.value = {
        ...bubbleStyle.value,
        '--tail-x': `${Math.round(Math.max(18, Math.min(bubbleRect.width - 18, tailX)))}px`,
      }
    })
  })
}

function clampMenuPos(clientX: number, clientY: number, menuW: number, menuH: number) {
  const vw = window.innerWidth
  const vh = window.innerHeight
  let x = clientX + 2
  let y = clientY + 2

  if (x + menuW + MENU_PAD > vw) {
    x = clientX - menuW - 2
  }
  if (x < MENU_PAD) x = MENU_PAD

  if (y + menuH + MENU_PAD > vh) {
    y = clientY - menuH - 2
  }
  if (y < MENU_PAD) y = MENU_PAD

  // 避免挡住头顶 speech bubble
  if (pet.showBubble && y < 72) {
    y = 72
    if (y + menuH + MENU_PAD > vh) {
      y = Math.max(MENU_PAD, vh - menuH - MENU_PAD)
    }
  }

  return { x, y }
}

async function onContextMenu(e: MouseEvent) {
  e.preventDefault()
  menuPosReady.value = false
  // 先用估算尺寸预定位，避免贴边时被窗口裁切
  menuPos.value = clampMenuPos(e.clientX, e.clientY, 108, rt.talking ? 168 : 168)
  menuVisible.value = true

  await nextTick()
  const el = menuEl.value
  if (!el) {
    menuPosReady.value = true
    return
  }
  const { width, height } = el.getBoundingClientRect()
  menuPos.value = clampMenuPos(e.clientX, e.clientY, width, height)
  menuPosReady.value = true
}

function closeMenu() {
  menuVisible.value = false
  menuPosReady.value = true
}

function onDblClick() {
  suppressClick = true
  if (clickTimer) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
  void openChat()
  setTimeout(() => {
    suppressClick = false
  }, 400)
}
</script>

<template>
  <div
    class="pet-shell"
    :class="{
      'side-panel-open': sidePanelOpen,
      dragging: isDragging && dragMoved,
    }"
    @pointerdown="onDragStart"
  >
    <div
      class="pet-area"
      @click.stop="onPetClick"
      @dblclick.stop="onDblClick"
      @contextmenu="onContextMenu"
    >
      <PetCanvas />
      <Transition name="bubble-pop">
        <div
          v-if="pet.showBubble"
          ref="bubbleEl"
          class="speech-bubble"
          :class="[
            pet.facing === 'left' ? 'speech-bubble--tr' : 'speech-bubble--tl',
            { 'speech-bubble--voice': pet.isVoiceBubbleActive() },
          ]"
          :style="bubbleStyle"
        >
          {{ pet.bubbleText }}
        </div>
      </Transition>
    </div>

    <div
      v-if="menuVisible"
      ref="menuEl"
      class="context-menu"
      :class="{ 'context-menu--pending': !menuPosReady }"
      :style="{ left: menuPos.x + 'px', top: menuPos.y + 'px' }"
      @click.stop
      @pointerdown.stop
    >
      <button type="button" @click.stop="onFeed">🍙 喂食</button>
      <button type="button" @click.stop="onPlay">🎾 玩耍</button>
      <button type="button" @pointerdown.stop @click.stop="openChatFromMenu">💬 聊天</button>
      <button v-if="!rt.talking" type="button" @click.stop="startVoiceFromMenu">🎤 语音对话</button>
      <button v-if="rt.talking" type="button" @click.stop="endVoiceFromMenu">🔇 结束对话</button>
      <button type="button" @pointerdown.stop @click.stop="openSettingsFromMenu">⚙️ 设置</button>
    </div>
  </div>
</template>

<style scoped>
.pet-shell {
  position: relative;
  width: 280px;
  height: 280px;
  background: transparent;
  overflow: visible;
  cursor: grab;
  touch-action: none;
}

.pet-shell.dragging,
.pet-shell.dragging .pet-area {
  cursor: grabbing;
}

.pet-shell.side-panel-open {
  width: 280px;
  height: 440px;
  align-self: flex-end;
  cursor: default;
}

.pet-shell.side-panel-open .pet-area {
  width: 280px;
  height: 280px;
}

.pet-area {
  width: 280px;
  height: 280px;
  flex-shrink: 0;
  position: relative;
  overflow: visible;
  z-index: 1;
}

.speech-bubble {
  position: absolute;
  top: 4px;
  left: 50%;
  transform: translateX(-50%);
  background: linear-gradient(145deg, rgba(255, 255, 255, 0.98) 0%, rgba(248, 250, 255, 0.96) 100%);
  padding: 10px 14px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.85);
  font-size: 13px;
  font-weight: 500;
  color: #2c3340;
  box-shadow:
    0 4px 20px rgba(88, 120, 180, 0.16),
    0 1px 3px rgba(0, 0, 0, 0.06);
  backdrop-filter: blur(8px);
  max-width: min(380px, calc(100vw - 24px));
  width: max-content;
  min-width: 0;
  box-sizing: border-box;
  overflow-wrap: anywhere;
  word-break: break-word;
  line-height: 1.55;
  letter-spacing: 0.01em;
  text-align: left;
  z-index: 30;
  pointer-events: none;
  white-space: pre-wrap;
}

.speech-bubble--voice {
  border-color: rgba(120, 160, 255, 0.35);
  box-shadow:
    0 6px 24px rgba(88, 120, 220, 0.2),
    0 0 0 1px rgba(120, 160, 255, 0.12);
}

.bubble-pop-enter-active {
  animation: bubble-in 0.28s cubic-bezier(0.22, 1, 0.36, 1);
}

.bubble-pop-leave-active {
  animation: bubble-out 0.22s ease-in forwards;
}

@keyframes bubble-in {
  from {
    opacity: 0;
    transform: translateX(-50%) translateY(6px) scale(0.94);
  }
  to {
    opacity: 1;
    transform: translateX(-50%) translateY(0) scale(1);
  }
}

@keyframes bubble-out {
  from {
    opacity: 1;
    transform: translateX(-50%) translateY(0) scale(1);
  }
  to {
    opacity: 0;
    transform: translateX(-50%) translateY(-4px) scale(0.96);
  }
}

.speech-bubble::after {
  content: '';
  position: absolute;
  bottom: -7px;
  left: var(--tail-x, 50%);
  transform: translateX(-50%);
  border: 7px solid transparent;
  border-top-color: rgba(252, 253, 255, 0.98);
  filter: drop-shadow(0 2px 2px rgba(88, 120, 180, 0.08));
}

.speech-bubble--tl::after {
  left: var(--tail-x, 58%);
}

.speech-bubble--tr::after {
  left: var(--tail-x, 42%);
}

.context-menu {
  position: fixed;
  background: white;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.16);
  overflow: hidden;
  z-index: 1000;
  width: max-content;
  pointer-events: auto;
}

.context-menu--pending {
  visibility: hidden;
  pointer-events: none;
}

.context-menu button {
  display: block;
  width: 100%;
  padding: 7px 10px;
  border: none;
  background: none;
  text-align: left;
  cursor: pointer;
  font-size: 12px;
  line-height: 1.3;
  white-space: nowrap;
}

.context-menu button:hover {
  background: #fff0f3;
}
</style>
