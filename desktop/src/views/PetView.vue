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
  openChatPanel,
  closeChatPanel,
  showChatPopupWindow,
  syncChatPopupPosition,
  isTauri,
  PET_WITH_CHAT_W,
  PET_WITH_CHAT_H,
} from '@/services/chatWindow'
import { getClientConfig, initClientConfig } from '@/config'
import PetCanvas from '@/components/pet/PetCanvas.vue'
import ChatPanel from '@/components/chat/ChatPanel.vue'

const { sidePanelOpen = false } = defineProps<{ sidePanelOpen?: boolean }>()

const pet = usePetStore()
const auth = useAuthStore()
const growth = useGrowthStore()
const rt = useRealtimeStore()
const menuVisible = ref(false)
const menuPos = ref({ x: 0, y: 0 })
const menuEl = ref<HTMLElement | null>(null)
const menuPosReady = ref(true)
const MENU_PAD = 6
const isDragging = ref(false)
const dragMoved = ref(false)
const didDragWindow = ref(false)
const chatExternal = ref(false)
const chatInline = ref(false)
const lastHeadlessBubbleIndex = ref(0)

const DRAG_THRESHOLD = 5

let dragWindow: ReturnType<typeof getCurrentWindow> | null = null
let clickTimer: ReturnType<typeof setTimeout> | null = null
let suppressClick = false
let roamer: PetRoamer | null = null

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
    if (pos && dragWindow) void dragWindow.setPosition(pos)
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
  scheduleWindowMove(dragWindowBase.x + dx, dragWindowBase.y + dy)
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
    if (chatExternal.value) void syncChatPopupPosition()
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
        rt.connected && (rt.talking || rt.processing),
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

onMounted(async () => {
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
      chatInline.value = false
      if (!rt.talking && !rt.processing) roamer?.resume()
    })
  } catch {
    // optional
  }
})

onUnmounted(() => {
  roamer?.stop()
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
      roamer?.resume()
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
    chatInline.value = false
    await closeChatPanel().catch(() => {})
    if (!rt.talking && !rt.processing) roamer?.resume()
  },
)

watch(
  () => rt.messages.length,
  (len) => {
    if (pet.isChatOpen || len === 0) return
    const last = rt.messages[len - 1]
    if (last?.role === 'assistant' && len > lastHeadlessBubbleIndex.value) {
      lastHeadlessBubbleIndex.value = len
      pet.showSpeechBubble(last.content, 8000)
    }
  },
)

watch(
  () => rt.userSpeaking,
  (speaking) => {
    if (pet.isChatOpen || !rt.talking) return
    if (speaking) {
      pet.showSpeechBubble(rt.partialText.trim() || '正在听…', 3000)
    }
  },
)

async function openChat() {
  menuVisible.value = false

  if (pet.isChatOpen) {
    pet.isChatOpen = false
    return
  }

  roamer?.pause()
  auth.syncFromStorage()
  chatExternal.value = false
  chatInline.value = false

  if (isTauri()) {
    const expanded = await openChatPanel()
    if (expanded) {
      chatInline.value = true
      pet.isChatOpen = true
      return
    }

    const popup = await showChatPopupWindow()
    if (popup) {
      chatExternal.value = true
      pet.isChatOpen = true
      return
    }

    pet.showSpeechBubble('聊天打开失败，请完全退出后重新运行')
    roamer?.resume()
    return
  }

  chatInline.value = true
  pet.isChatOpen = true
}

function openChatFromMenu() {
  closeMenu()
  void openChat()
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
    roamer?.pause()

    if (rt.talking) {
      if (rt.resting) {
        pet.showSpeechBubble('我在听，主人说~', 2500)
      }
      return
    }

    await initClientConfig().catch(() => {})
    if (!getClientConfig().realtimeEnabled) {
      pet.showSpeechBubble('请双击打开聊天，用打字跟我聊~')
      roamer?.resume()
      return
    }

    pet.setAnimation('happy')
    pet.showSpeechBubble(pet.getWakeGreeting())

    try {
      await rt.connect()
      await rt.startTalk()
      if (!rt.talking && rt.statusText) {
        pet.showSpeechBubble(rt.statusText, 6000)
        pet.syncAnimationFromState()
        roamer?.resume()
      }
    } catch {
      pet.showSpeechBubble('无法启动麦克风，请检查权限')
      pet.syncAnimationFromState()
      roamer?.resume()
    }
  }, 200)
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
    pet.updateLifeState(result.state)
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
    pet.updateLifeState(result.state)
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

function openSettingsFromMenu() {
  closeMenu()
  growth.openSettings()
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
  if (pet.showBubble && y < 80) {
    y = 80
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
  menuPos.value = clampMenuPos(e.clientX, e.clientY, 108, rt.talking ? 168 : 136)
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
      'chat-open': chatInline,
      'side-panel-open': sidePanelOpen,
      dragging: isDragging && dragMoved,
    }"
    :style="chatInline && !isTauri() ? {
      width: PET_WITH_CHAT_W + 'px',
      height: PET_WITH_CHAT_H + 'px',
    } : undefined"
    @pointerdown="onDragStart"
  >
    <div
      class="pet-area"
      @click.stop="onPetClick"
      @dblclick.stop="onDblClick"
      @contextmenu="onContextMenu"
    >
      <PetCanvas />

      <div
        v-if="pet.showBubble"
        class="speech-bubble"
        :class="pet.facing === 'left' ? 'speech-bubble--tr' : 'speech-bubble--tl'"
      >
        {{ pet.bubbleText }}
      </div>
    </div>

    <ChatPanel v-if="chatInline" class="chat-side" @pointerdown.stop />

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
      <button v-if="rt.talking" type="button" @click.stop="endVoiceFromMenu">🔇 结束对话</button>
      <button type="button" @click.stop="openSettingsFromMenu">⚙️ 设置</button>
    </div>
  </div>
</template>

<style scoped>
.pet-shell {
  position: relative;
  width: 200px;
  height: 220px;
  background: transparent;
  overflow: hidden;
  cursor: grab;
  touch-action: none;
}

.pet-shell.dragging,
.pet-shell.dragging .pet-area {
  cursor: grabbing;
}

.pet-shell.side-panel-open {
  width: 200px;
  height: 100%;
  min-height: 440px;
  cursor: default;
}

.pet-shell.side-panel-open .pet-area {
  height: 100%;
}

.pet-shell.chat-open {
  display: flex;
  flex-direction: row;
  align-items: flex-end;
  gap: 8px;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.pet-area {
  width: 200px;
  height: 220px;
  flex-shrink: 0;
  position: relative;
}

.chat-side {
  width: 320px;
  height: 440px;
  flex-shrink: 0;
}

.speech-bubble {
  position: absolute;
  top: 0;
  background: rgba(255, 255, 255, 0.95);
  padding: 6px 10px;
  border-radius: 14px;
  font-size: 12px;
  color: #333;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.12);
  max-width: 118px;
  line-height: 1.35;
  text-align: left;
  z-index: 10;
  pointer-events: none;
}

/* 朝右时气泡在头部左上，不挡脸 */
.speech-bubble--tl {
  left: 4px;
  right: auto;
}

.speech-bubble--tl::after {
  content: '';
  position: absolute;
  bottom: -6px;
  right: 20px;
  left: auto;
  transform: none;
  border: 6px solid transparent;
  border-top-color: rgba(255, 255, 255, 0.95);
}

/* 朝左时气泡在头部右上 */
.speech-bubble--tr {
  right: 4px;
  left: auto;
}

.speech-bubble--tr::after {
  content: '';
  position: absolute;
  bottom: -6px;
  left: 20px;
  right: auto;
  transform: none;
  border: 6px solid transparent;
  border-top-color: rgba(255, 255, 255, 0.95);
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
