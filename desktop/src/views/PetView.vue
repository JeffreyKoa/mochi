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
import { warmUpMicrophoneAccess, micPermissionDeniedMessage } from '@/utils/micPermission'
import { onLipSync } from '@/services/voice'
import {
  claimVoiceOwner,
  getStoredVoiceOwner,
  isChatWindowVisible,
  releaseVoiceOwner,
  setupVoiceOwnerListener,
} from '@/services/voiceSessionOwner'
import { invoke } from '@tauri-apps/api/core'
import { listen, type UnlistenFn } from '@tauri-apps/api/event'

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
const isDragging = ref(false)
const dragMoved = ref(false)
const didDragWindow = ref(false)
const chatExternal = ref(false)
const lastHeadlessBubbleIndex = ref(0)

const DRAG_THRESHOLD = 5

/** 单击 Mochi 唤醒时的唯一提示语；之后仅展示 ASR 实时识别文字。 */
const PET_WAKE_GREETING = '在呢，主人！'

/** manual wake 失败时给用户可见反馈（避免「点了没反应」）。 */
async function showWakeFailure(reason: string | undefined) {
  switch (reason) {
    case 'voiceprint_missing':
      pet.showSpeechBubble('请先在设置中录入主人声纹~', 4000)
      break
    case 'disconnected':
      pet.showSpeechBubble('连接断开，正在重连…', 3000)
      await rt.connectIfOwner().catch(() => {})
      break
    case 'not_owner':
      if (!rt.statusText) {
        pet.showSpeechBubble('我没听清是不是你，请再说一遍~', 2500)
      }
      break
    case 'not_ready':
      pet.showSpeechBubble('还没准备好，请再点一次~', 2500)
      break
    default:
      break
  }
}

let dragWindow: ReturnType<typeof getCurrentWindow> | null = null
let clickTimer: ReturnType<typeof setTimeout> | null = null
let suppressClick = false
let roamer: PetRoamer | null = null
let unlistenPetMoved: (() => void) | null = null
let unlistenVoiceOwner: UnlistenFn | null = null

let dragPointerId = -1
let dragWindowBase = { x: 0, y: 0 }
let dragBaseReady = false
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
  if (!isDragging.value || e.pointerId !== dragPointerId || !dragWindow || !dragBaseReady) return
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
    void saveWindowPosition(dragWindow)
    if (isTauri()) {
      void invoke('sync_chat_beside_pet').catch(() => {})
    }
  }

  setTimeout(() => {
    dragMoved.value = false
    didDragWindow.value = false
    dragBaseReady = false
    if (!pet.isChatOpen && !rt.talking && !rt.processing && !menuVisible.value) roamer?.resume()
  }, 30)
}

async function onDragStart(e: PointerEvent) {
  if (e.button !== 0 || !dragWindow) return

  isDragging.value = true
  dragMoved.value = false
  didDragWindow.value = false
  dragBaseReady = false
  dragPointerId = e.pointerId
  dragPointerStart = { x: e.screenX, y: e.screenY }
  roamer?.pause()
  closeMenu()

  try {
    const pos = await dragWindow.outerPosition()
    dragWindowBase = { x: pos.x, y: pos.y }
    dragBaseReady = true
  } catch {
    dragBaseReady = false
    isDragging.value = false
    dragPointerId = -1
    roamer?.resume()
    return
  }

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
        menuVisible.value,
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
  () => [pet.showBubble, pet.bubbleText, pet.facing, pet.currentAnimation] as const,
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

  if (isTauri() && auth.isLoggedIn) {
    void warmUpMicrophoneAccess()
  }

  if (auth.isLoggedIn) {
    await initClientConfig().catch(() => {})
    if (isTauri()) {
      rt.setVoiceWindow('pet')
      unlistenVoiceOwner = await setupVoiceOwnerListener('pet', {
        onAcquire: () => rt.connectIfOwner(),
        onYield: () => rt.yieldVoiceConnection(),
      })
      const chatVisible = await isChatWindowVisible()
      if (!chatVisible && !pet.isChatOpen && getStoredVoiceOwner() !== 'chat') {
        await claimVoiceOwner('pet')
      }
    } else {
      rt.setVoiceWindow('inline')
    }
  }

  try {
    const { listen } = await import('@tauri-apps/api/event')
    await listen('chat-closed', async () => {
      pet.isChatOpen = false
      chatExternal.value = false
      pet.chatInline = false
      setPopupChatFollowsPet(false)
      if (auth.isLoggedIn && isTauri()) {
        await claimVoiceOwner('pet')
        await rt.connectIfOwner()
      }
      if (!rt.talking && !rt.processing) roamer?.resume()
    })
    await listen('side-panel-closed', async (event) => {
      const mode = (event.payload as { mode?: string } | undefined)?.mode
      if (mode === 'settings') growth.closeSettings()
      if (mode === 'chat' || mode === 'settings' || !mode) {
        if (mode !== 'settings') {
          pet.isChatOpen = false
          pet.chatInline = false
          chatExternal.value = false
        }
        if (auth.isLoggedIn && isTauri()) {
          await claimVoiceOwner('pet')
          await rt.connectIfOwner()
        }
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
  unlistenVoiceOwner?.()
  unlistenVoiceOwner = null
  roamer?.stop()
  unlistenPetMoved?.()
  unlistenPetMoved = null
  window.removeEventListener('pointermove', onWindowPointerMove)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerUp)
  window.removeEventListener('pointerdown', onDocumentPointerDown, true)
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
    if (pet.isChatOpen || !rt.talking || pet.isReminderBubbleActive()) return
    if (!rt.userSpeaking && !rt.processing) return
    const trimmed = text.trim()
    if (!trimmed) return
    pet.showPersistentBubble(`“${trimmed}”`)
  },
)

watch(
  () => rt.userSpeaking,
  (speaking) => {
    if (pet.isChatOpen || !rt.talking || pet.isReminderBubbleActive()) return
    if (speaking && rt.partialText.trim()) {
      pet.showPersistentBubble(`“${rt.partialText.trim()}”`)
    }
  },
)

async function closeChatSurface(collapse = true) {
  pet.isChatOpen = false
  chatExternal.value = false
  pet.chatInline = false
  setPopupChatFollowsPet(false)
  if (isTauri()) {
    if (getStoredVoiceOwner() === 'chat') {
      await rt.yieldVoiceConnection()
      await releaseVoiceOwner('chat')
    }
    await closeChatPanel(collapse)
    try {
      const { emit } = await import('@tauri-apps/api/event')
      await emit('chat-closed', {})
      await emit('side-panel-closed', { mode: 'chat' })
    } catch {
      // optional
    }
  }
}

async function openChat() {
  closeMenu(false)

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

  pet.hideSpeechBubble()
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

  if (isTauri()) {
    rt.setVoiceWindow('pet')
    await claimVoiceOwner('pet')
  }

  pet.setAnimation('happy')

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
    // 单击即问候 + 立即进入实时聆听（manual wake → 流式 ASR）
    if (rt.resting) {
      const wake = await rt.wakeListening({ manual: true })
      if (wake.ok) {
        pet.showPersistentBubble(PET_WAKE_GREETING)
      } else {
        await showWakeFailure(wake.reason)
      }
    }
    return true
  } catch {
    pet.showSpeechBubble(micPermissionDeniedMessage(), 8000)
    pet.syncAnimationFromState()
    roamer?.resume()
    return false
  }
}

async function handlePetTap() {
  if (dragMoved.value || suppressClick) return
  if (pet.bootFailed) {
    pet.retryBoot()
    return
  }

  // 连接已断但 talking 未清理 → 重置后重试
  if (rt.talking && !rt.connected) {
    await rt.endConversation()
    await startVoiceInteraction()
    return
  }

  if (rt.talking) {
    if (rt.resting) {
      const wake = await rt.wakeListening({ manual: true })
      if (wake.ok) {
        pet.showPersistentBubble(PET_WAKE_GREETING)
      } else {
        await showWakeFailure(wake.reason)
      }
      return
    }
    if (rt.userSpeaking) {
      rt.submitUtterance(true)
      return
    }
    if (rt.processing) {
      return
    }
    return
  }

  await startVoiceInteraction()
}

async function onPetClick() {
  if (didDragWindow.value || suppressClick) return
  if (clickTimer) clearTimeout(clickTimer)
  clickTimer = setTimeout(async () => {
    clickTimer = null
    await handlePetTap()
  }, 80)
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
  closeMenu(false)
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
  closeMenu(false)
  roamer?.pause()
  try {
    const result = await interactWithRetry('play')
    pet.updateLifeState(result.state as Parameters<typeof pet.updateLifeState>[0])
    pet.setAnimation('happy')
    pet.triggerHappyBurst()
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

  // 打开设置：必须关闭聊天态，避免侧栏仍显示 ChatPanel
  pet.isChatOpen = false
  pet.chatInline = false
  chatExternal.value = false
  setPopupChatFollowsPet(false)
  if (isTauri()) {
    await hideSidePanelPopup()
    await claimVoiceOwner('pet')
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

  // Mochi 头部在画布上方（BODY_CY≈18），气泡放底部腿/尾区域，避免挡脸
  bubbleStyle.value = {
    left: '12px',
    right: '12px',
    bottom: '12px',
    top: 'auto',
    transform: 'none',
    width: 'fit-content',
    maxWidth: '256px',
    margin: '0 auto',
  }
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

  // 避免挡住头顶 speech bubble（气泡在头部上方，预留更高空间）
  if (pet.showBubble && y < 48) {
    y = 48
    if (y + menuH + MENU_PAD > vh) {
      y = Math.max(MENU_PAD, vh - menuH - MENU_PAD)
    }
  }

  return { x, y }
}

function onDocumentPointerDown(e: PointerEvent) {
  if (!menuVisible.value) return
  const el = menuEl.value
  if (el && !el.contains(e.target as Node)) {
    closeMenu()
  }
}

async function onContextMenu(e: MouseEvent) {
  e.preventDefault()
  roamer?.pause()
  menuPosReady.value = false
  // 先用估算尺寸预定位，避免贴边时被窗口裁切
  menuPos.value = clampMenuPos(e.clientX, e.clientY, 108, rt.talking ? 168 : 168)
  menuVisible.value = true
  window.addEventListener('pointerdown', onDocumentPointerDown, true)

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

function closeMenu(resumeRoam = true) {
  menuVisible.value = false
  menuPosReady.value = true
  window.removeEventListener('pointerdown', onDocumentPointerDown, true)
  if (resumeRoam && !isDragging.value && !pet.isChatOpen && !rt.talking && !rt.processing) {
    roamer?.resume()
  }
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
            bubbleStyle.right !== 'auto' ? 'speech-bubble--tl' : 'speech-bubble--tr',
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
  bottom: 12px;
  top: auto;
  left: 12px;
  right: 12px;
  transform: none;
  margin: 0 auto;
  background: linear-gradient(145deg, rgba(255, 255, 255, 0.98) 0%, rgba(248, 250, 255, 0.96) 100%);
  padding: 8px 12px;
  border-radius: 14px;
  border: 1px solid rgba(255, 255, 255, 0.88);
  font-size: 12px;
  font-weight: 500;
  color: #2c3340;
  box-shadow:
    0 6px 20px rgba(88, 120, 180, 0.18),
    0 1px 4px rgba(0, 0, 0, 0.08);
  backdrop-filter: blur(10px);
  width: fit-content;
  width: max-content;
  max-width: 240px;
  min-width: 42px;
  max-height: 96px;
  overflow-y: auto;
  box-sizing: border-box;
  overflow-wrap: anywhere;
  word-break: break-word;
  line-height: 1.45;
  letter-spacing: 0.01em;
  text-align: left;
  z-index: 30;
  pointer-events: auto;
  white-space: pre-wrap;
}

.speech-bubble::-webkit-scrollbar {
  width: 3px;
}

.speech-bubble::-webkit-scrollbar-thumb {
  background: rgba(140, 160, 200, 0.4);
  border-radius: 3px;
}

.speech-bubble::-webkit-scrollbar-track {
  background: transparent;
}

.speech-bubble::after {
  content: '';
  position: absolute;
  top: -6px;
  left: 50%;
  transform: translateX(-50%);
  border: 6px solid transparent;
  border-bottom-color: rgba(252, 253, 255, 0.98);
  filter: drop-shadow(0 -1px 2px rgba(88, 120, 180, 0.08));
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
