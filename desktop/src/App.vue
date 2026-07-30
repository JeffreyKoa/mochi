<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { useAuthStore } from '@/stores/authStore'
import { usePetStore, type PetLifecycle, type PetPersonality } from '@/stores/petStore'
import { useRealtimeStore } from '@/stores/realtimeStore'
import { useGrowthStore } from '@/stores/growthStore'
import { getPet, getLifeState, getChatHistory, initClientConfig, AuthError, ApiError } from '@/services/api'
import { healthMonitor } from '@/services/healthMonitor'
import { startActivityHeartbeat, stopActivityHeartbeat } from '@/services/activityHeartbeat'
import { stopAmbientMic } from '@/services/ambientMic'
import { wsManager } from '@/services/ws'
import { handleProactiveMessage } from '@/services/proactiveHandler'
import { listenProactive } from '@/services/proactiveSync'
import {
  ensurePetWindowVisible,
  initPetWindowChrome,
  isPetWindowLabel,
  isTauri,
  sidePanelOnLeft,
  syncBrowserSidePanelPlacement,
  syncPanelShellLayout,
  consumeSidePanelPending,
} from '@/services/chatWindow'
import { setLoginLayout, setPetOnlyLayout, PET_WITH_SIDE_W, PET_WITH_SIDE_H } from '@/services/windowLayout'
import LoginView from '@/views/LoginView.vue'
import OnboardingView from '@/views/OnboardingView.vue'
import PetView from '@/views/PetView.vue'
import ChatPanel from '@/components/chat/ChatPanel.vue'
import SettingsPanel from '@/components/growth/SettingsPanel.vue'
import AdoptView from '@/views/AdoptView.vue'

const auth = useAuthStore()
const pet = usePetStore()
const rt = useRealtimeStore()
const growth = useGrowthStore()
const ready = ref(false)
const loading = ref(true)
const loadError = ref('')
const showOnboarding = ref(false)
const showAdopt = ref(false)
const winLabel = ref('browser')
const wsInitialized = ref(false)
const popupPanelMode = ref<'chat' | 'settings' | null>(null)
let unlistenProactive: (() => void) | null = null

async function applyPopupPanelMode(mode: 'chat' | 'settings', token?: string | null) {
  auth.syncFromStorage()
  if (token && !auth.isLoggedIn) {
    auth.setToken(token)
  }
  popupPanelMode.value = mode
  if (!auth.isLoggedIn) {
    console.warn('[panel] not logged in in popup window')
    return
  }
  if (mode === 'settings') {
    growth.openSettings()
  } else {
    await loadUserData()
  }
}

const isBrowserDev = computed(() => !isTauri())
const isChatWindow = computed(() => winLabel.value === 'chat')
const isPetShell = computed(() => isBrowserDev.value || isPetWindowLabel(winLabel.value))
const sidePanelOpen = computed(
  () => showOnboarding.value || growth.showSettings || showAdopt.value,
)
/** Inline expand only for onboarding/adopt in Tauri; chat/settings use popup. */
const shellExpanded = computed(() => {
  if (isTauri() && isPetShell.value && !isBrowserDev.value) {
    return showOnboarding.value || showAdopt.value
  }
  return sidePanelOpen.value || pet.isChatOpen
})
const useInlineSidePanel = computed(() => isBrowserDev.value)

async function applySidePanelLayout() {
  if (isChatWindow.value) return
  if (isBrowserDev.value) {
    if (shellExpanded.value) syncBrowserSidePanelPlacement()
    return
  }
  await syncPanelShellLayout(shellExpanded.value)
}

watch(
  () => shellExpanded.value,
  () => {
    void applySidePanelLayout()
  },
)

function friendlyLoadError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.kind === 'network') return '网络有点卡，点我一下重试~'
    if (e.status === 503 || e.status === 500) return '后端有点忙，点我一下重试~'
  }
  if (e instanceof Error && (e.message.includes('500') || e.message.includes('503') || e.message.includes('重试'))) {
    return '连接不太稳，点我一下重试~'
  }
  return e instanceof Error ? e.message : '加载失败，点我一下重试~'
}

function setupWs() {
  if (!isPetShell.value || isChatWindow.value) return
  if (!wsInitialized.value) {
    wsInitialized.value = true
    wsManager.on('state_update', (data: unknown) => {
      const d = data as { state: typeof pet.lifeState; animation: string }
      pet.updateLifeState(d.state)
      if (d.animation) pet.setAnimation(d.animation as typeof pet.currentAnimation)
    })
    wsManager.on('proactive_message', (data: unknown) => {
      const d = data as { message: string; animation: string }
      handleProactiveMessage({ message: d.message, animation: d.animation }, { priority: true })
      rt.appendAssistantMessage(d.message)
    })
    wsManager.on('life_stage_changed', (data: unknown) => {
      const d = data as Partial<PetLifecycle> & { life_stage_label?: string }
      pet.updateLifecycle({
        life_stage: d.life_stage,
        life_stage_label: d.life_stage_label,
        age_days: d.age_days,
        age_years: d.age_years,
        age_days_in_year: d.age_days_in_year,
        remaining_days: d.remaining_days,
        is_alive: d.is_alive,
      })
      if (d.life_stage_label) {
        pet.showSpeechBubble(`我进入${d.life_stage_label}啦~`, 6000)
      }
    })
  }
  wsManager.connect(true)
}

function startHealthWatch() {
  if (healthMonitor.watching) return
  pet.setBootFailed(true)
  pet.showPersistentBubble('网络有点卡，我在自动重连…')
  healthMonitor.start(
    () => {
      loadError.value = ''
      pet.hideSpeechBubble()
      pet.showSpeechBubble('连上了~')
      void loadUserData()
    },
    (_attempt, up) => {
      if (!up && _attempt >= 120) {
        pet.showPersistentBubble('还是连不上，点我一下再试~')
      }
    },
  )
}

function handleLoadFailure(e: unknown) {
  if (e instanceof AuthError) {
    healthMonitor.stop()
    pet.setBootFailed(false)
    pet.hideSpeechBubble()
    auth.logout()
    return
  }
  const msg = friendlyLoadError(e)
  loadError.value = msg
  pet.setBootFailed(true)
  pet.showPersistentBubble(msg)
  if (e instanceof ApiError && (e.kind === 'network' || e.kind === 'server')) {
    startHealthWatch()
  }
}

async function retryLoadUserData() {
  pet.showSpeechBubble('再试一次~', 2000)
  const ok = await healthMonitor.poke(() => {
    loadError.value = ''
    void loadUserData()
  })
  if (!ok) {
    startHealthWatch()
  }
}

async function loadUserData() {
  loadError.value = ''
  showAdopt.value = false
  try {
    const petData = (await getPet()) as {
      name: string
      sku_id?: string
      needs_adopt?: boolean
      sku?: { sku_id?: string; name?: string; skin?: unknown; breed_name?: string }
      species?: string
      breed?: string
      life_stage?: string
      life_stage_label?: string
      stage_hint?: string
      gender?: string
      personality?: PetPersonality
      age_days?: number
      age_years?: number
      age_days_in_year?: number
      remaining_days?: number
      max_days?: number
      is_alive?: boolean
      life_state?: Parameters<typeof pet.updateLifeState>[0]
    }

    if (petData.needs_adopt || !petData.sku_id) {
      showAdopt.value = true
      await applySidePanelLayout()
      loading.value = false
      return
    }

    pet.petName = petData.name
    pet.gender = petData.gender === 'male' ? 'male' : 'female'
    pet.personality = petData.personality ?? {}
    pet.stageHint = petData.stage_hint ?? ''
    pet.applySkinFromSKU(petData.sku)
    pet.updateLifecycle({
      species: petData.species ?? 'cat',
      breed: petData.breed ?? '',
      life_stage: petData.life_stage ?? 'newborn',
      life_stage_label: petData.life_stage_label ?? '刚出生',
      age_days: petData.age_days ?? 0,
      age_years: petData.age_years ?? 0,
      age_days_in_year: petData.age_days_in_year ?? 0,
      remaining_days: petData.remaining_days ?? 6570,
      max_days: petData.max_days ?? 6570,
      is_alive: petData.is_alive ?? true,
    })
    if (petData.life_state) {
      pet.updateLifeState(petData.life_state)
    } else {
      try {
        const state = await getLifeState()
        pet.updateLifeState(state as Parameters<typeof pet.updateLifeState>[0])
      } catch (e) {
        console.warn('[load] life state optional, skipped', e)
      }
    }
    pet.syncAnimationFromState()

    try {
      const history = (await getChatHistory()) as Array<{ role: string; content: string; created_at?: string }> | null
      rt.loadHistory(
        (history ?? []).map((m) => ({
          role: m.role as 'user' | 'assistant',
          content: m.content,
          createdAt: m.created_at,
        })),
      )
    } catch (e) {
      console.warn('[load] chat history optional, skipped', e)
    }

    setupWs()
    if (!isChatWindow.value) {
      void rt.ensurePushConnected()
    }
    startActivityHeartbeat()
    void rt.initAmbientPresence().catch((e) => {
      console.warn('[ambient] presence init skipped', e)
    })
    healthMonitor.stop()
    loadError.value = ''
    pet.setBootFailed(false)
    pet.hideSpeechBubble()

    try {
      await growth.fetchBondAndBrief()
      showOnboarding.value = growth.onboardingRequired
      await applySidePanelLayout()
    } catch (e) {
      console.warn('[load] bond/brief optional, skipped', e)
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      showAdopt.value = true
      await applySidePanelLayout()
      loading.value = false
      return
    }
    console.error('load user data failed', e)
    handleLoadFailure(e)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    winLabel.value = getCurrentWindow().label
  } catch {
    winLabel.value = 'browser'
  }

  ready.value = true
  pet.registerBootRetry(() => void retryLoadUserData())

  if (isChatWindow.value) {
    auth.syncFromStorage()
    await initClientConfig().catch((e) => console.warn('[chat] config', e))
    loading.value = false
    unlistenProactive = await listenProactive((payload) => {
      rt.appendAssistantMessage(payload.message)
    })
    try {
      const { listen } = await import('@tauri-apps/api/event')
      await listen('side-panel-opened', async (event) => {
        const payload = event.payload as { mode?: 'chat' | 'settings'; token?: string | null } | undefined
        await applyPopupPanelMode(payload?.mode ?? 'chat', payload?.token)
      })
      await listen('chat-opened', async (event) => {
        const payload = event.payload as { mode?: 'chat' | 'settings'; token?: string | null } | undefined
        await applyPopupPanelMode(payload?.mode ?? 'chat', payload?.token)
      })
      await listen('side-panel-side-changed', (event) => {
        sidePanelOnLeft.value = !!event.payload
      })
    } catch (e) {
      console.warn('[chat] init listener failed', e)
    }
    const pending = consumeSidePanelPending()
    if (pending) {
      await applyPopupPanelMode(pending.mode, pending.token)
    } else if (auth.isLoggedIn) {
      void loadUserData()
    }
    return
  }

  await initClientConfig().catch((e) => console.warn('[init] config', e))
  await initPetWindowChrome()
  await ensurePetWindowVisible()

  if (auth.isLoggedIn) {
    loading.value = false
    void setPetOnlyLayout()
    void loadUserData()
  } else {
    loading.value = false
    await setLoginLayout()
  }
})

watch(
  () => auth.isLoggedIn,
  async (loggedIn) => {
    if (!ready.value || isChatWindow.value) return
    if (loggedIn) {
      loadError.value = ''
      await setPetOnlyLayout()
      loading.value = false
      void loadUserData()
    } else {
      pet.isChatOpen = false
      loading.value = false
      healthMonitor.stop()
      stopActivityHeartbeat()
      void stopAmbientMic()
      await setLoginLayout()
    }
  },
)

async function onLoginSuccess() {
  loadError.value = ''
  healthMonitor.stop()
  await setPetOnlyLayout()
  loading.value = false
  void loadUserData()
}

function onOnboardingDone() {
  showOnboarding.value = false
  void applySidePanelLayout()
}

async function onAdopted() {
  showAdopt.value = false
  loading.value = true
  await loadUserData()
}

function onAdoptLogout() {
  showAdopt.value = false
  growth.closeSettings()
  showOnboarding.value = false
  wsManager.disconnect()
  wsInitialized.value = false
  stopActivityHeartbeat()
  void stopAmbientMic()
  void setLoginLayout()
}

onUnmounted(() => {
  healthMonitor.stop()
  stopActivityHeartbeat()
  void stopAmbientMic()
  unlistenProactive?.()
  unlistenProactive = null
})
</script>

<template>
  <div class="app-root" :class="{ 'app-root--chat-popup': isChatWindow }">
    <!-- Vite browser dev -->
    <template v-if="isBrowserDev">
      <LoginView v-if="ready && !auth.isLoggedIn" @success="onLoginSuccess" />
      <template v-else-if="ready && auth.isLoggedIn">
        <div
          class="dual-shell"
          :class="{
            'dual-shell--expanded': shellExpanded,
            'dual-shell--panel-left': shellExpanded && sidePanelOnLeft,
          }"
          :style="shellExpanded && isBrowserDev ? {
            width: PET_WITH_SIDE_W + 'px',
            height: PET_WITH_SIDE_H + 'px',
          } : undefined"
        >
          <PetView :side-panel-open="useInlineSidePanel && (sidePanelOpen || pet.chatInline)" />
          <AdoptView v-if="showAdopt" @adopted="onAdopted" @logout="onAdoptLogout" />
          <OnboardingView v-else-if="showOnboarding" @done="onOnboardingDone" />
          <SettingsPanel v-else-if="useInlineSidePanel && growth.showSettings" />
          <ChatPanel v-else-if="useInlineSidePanel && pet.isChatOpen && pet.chatInline" docked @pointerdown.stop />
        </div>
        <p v-if="loadError && !showOnboarding && !showAdopt" class="load-error">{{ loadError }}</p>
      </template>
    </template>

    <!-- Tauri side panel popup (chat / settings) -->
    <template v-else-if="isChatWindow && ready">
      <SettingsPanel v-if="popupPanelMode === 'settings'" :class="{ 'panel-on-left': sidePanelOnLeft }" />
      <ChatPanel v-else floating :class="{ 'panel-on-left': sidePanelOnLeft }" />
    </template>

    <!-- Tauri pet window -->
    <template v-else-if="isPetShell && ready">
      <LoginView v-if="!auth.isLoggedIn" @success="onLoginSuccess" />
      <template v-else>
        <div
          class="dual-shell"
          :class="{
            'dual-shell--expanded': shellExpanded,
            'dual-shell--panel-left': shellExpanded && sidePanelOnLeft,
          }"
        >
          <PetView :side-panel-open="useInlineSidePanel && (sidePanelOpen || pet.chatInline)" />
          <AdoptView v-if="showAdopt" @adopted="onAdopted" @logout="onAdoptLogout" />
          <OnboardingView v-else-if="showOnboarding" @done="onOnboardingDone" />
          <SettingsPanel v-else-if="useInlineSidePanel && growth.showSettings" />
          <ChatPanel v-else-if="useInlineSidePanel && pet.isChatOpen && pet.chatInline" docked @pointerdown.stop />
        </div>
        <p v-if="loading && !showOnboarding && !growth.showSettings && !showAdopt" class="boot-hint">Mochi 醒来中...</p>
        <p v-if="loadError" class="load-error">{{ loadError }}</p>
      </template>
    </template>
  </div>
</template>

<style scoped>
.app-root {
  width: 100%;
  height: 100%;
  background: transparent;
  overflow: visible;
}

.app-root--chat-popup {
  background: transparent;
  overflow: visible;
}

.dev-shell {
  display: flex;
  flex-direction: row;
  background: transparent;
}

.dual-shell {
  display: flex;
  flex-direction: row;
  align-items: flex-end;
  gap: 8px;
  width: 100%;
  height: 100%;
  background: transparent;
  overflow: visible;
}

.dual-shell--expanded {
  width: 100%;
  height: 100%;
}

.dual-shell--panel-left {
  flex-direction: row-reverse;
}

.boot-hint {
  position: fixed;
  bottom: 8px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.92);
  padding: 4px 10px;
  border-radius: 10px;
  font-size: 11px;
  color: #666;
  z-index: 100;
  white-space: nowrap;
  pointer-events: none;
}

.load-error {
  position: fixed;
  top: 8px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 243, 205, 0.95);
  color: #856404;
  padding: 6px 12px;
  border-radius: 8px;
  font-size: 12px;
  z-index: 200;
  max-width: 90%;
  text-align: center;
  pointer-events: none;
}
</style>
