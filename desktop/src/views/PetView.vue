<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { PhysicalPosition } from '@tauri-apps/api/dpi'
import { useAuthStore } from '@/stores/authStore'
import { usePetStore } from '@/stores/petStore'
import { interact } from '@/services/api'
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
import PetCanvas from '@/components/pet/PetCanvas.vue'
import ChatPanel from '@/components/chat/ChatPanel.vue'

const pet = usePetStore()
const auth = useAuthStore()
const menuVisible = ref(false)
const menuPos = ref({ x: 0, y: 0 })
const isDragging = ref(false)
const dragMoved = ref(false)
const didDragWindow = ref(false)
const chatExternal = ref(false)
const chatInline = ref(false)

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
    if (!pet.isChatOpen) roamer?.resume()
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
  if (!dragWindow || roamer) return
  roamer = new PetRoamer()
  roamer.start(dragWindow, {
    isPaused: () => !canRoam(pet.lifeState.energy, pet.isChatOpen, isDragging.value),
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

  try {
    const { listen } = await import('@tauri-apps/api/event')
    await listen('chat-closed', () => {
      pet.isChatOpen = false
      chatExternal.value = false
      chatInline.value = false
      roamer?.resume()
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
    roamer?.resume()
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
    pet.setAnimation('happy')
    try {
      const result = await interact('touch')
      pet.updateLifeState(result.state)
      pet.setAnimation(result.animation)
      pet.showSpeechBubble('嘿嘿~')
      setTimeout(() => {
        pet.syncAnimationFromState()
        roamer?.resume()
      }, 2000)
    } catch (e) {
      const msg = e instanceof Error ? e.message : '连接失败'
      pet.showSpeechBubble(msg.includes('登录') ? msg : '连接失败，请检查后端')
      pet.setAnimation('idle')
      setTimeout(() => roamer?.resume(), 2000)
    }
  }, 200)
}

async function onFeed() {
  menuVisible.value = false
  roamer?.pause()
  try {
    const result = await interact('feed')
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
    const result = await interact('play')
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

function onContextMenu(e: MouseEvent) {
  e.preventDefault()
  let x = e.clientX + 2
  let y = e.clientY + 2
  // 避免挡住头顶 speech bubble
  if (pet.showBubble && y < 80) y = 80
  menuPos.value = { x, y }
  menuVisible.value = true
}

function closeMenu() {
  menuVisible.value = false
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
    :class="{ 'chat-open': chatInline, dragging: isDragging && dragMoved }"
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

      <div v-if="pet.showBubble" class="speech-bubble">
        {{ pet.bubbleText }}
      </div>
    </div>

    <ChatPanel v-if="chatInline" class="chat-side" @pointerdown.stop />

    <div
      v-if="menuVisible"
      class="context-menu"
      :style="{ left: menuPos.x + 'px', top: menuPos.y + 'px' }"
      @click.stop
      @pointerdown.stop
    >
      <button type="button" @click.stop="onFeed">🍙 喂食</button>
      <button type="button" @click.stop="onPlay">🎾 玩耍</button>
      <button type="button" @pointerdown.stop @click.stop="openChatFromMenu">💬 聊天</button>
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
  top: 4px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.95);
  padding: 6px 12px;
  border-radius: 14px;
  font-size: 12px;
  color: #333;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.12);
  max-width: 180px;
  text-align: center;
  z-index: 10;
  pointer-events: none;
}

.speech-bubble::after {
  content: '';
  position: absolute;
  bottom: -6px;
  left: 50%;
  transform: translateX(-50%);
  border: 6px solid transparent;
  border-top-color: white;
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
