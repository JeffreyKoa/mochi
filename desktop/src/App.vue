<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { useAuthStore } from '@/stores/authStore'
import { usePetStore, type PetLifecycle } from '@/stores/petStore'
import { useRealtimeStore } from '@/stores/realtimeStore'
import { useGrowthStore } from '@/stores/growthStore'
import { getPet, getLifeState, getChatHistory, initClientConfig, AuthError, ApiError } from '@/services/api'
import { healthMonitor } from '@/services/healthMonitor'
import { wsManager } from '@/services/ws'
import { handleProactiveMessage } from '@/services/proactiveHandler'
import { listenProactive } from '@/services/proactiveSync'
import {
  ensurePetWindowVisible,
  initPetWindowChrome,
  isPetWindowLabel,
  isTauri,
} from '@/services/chatWindow'
import { setLoginLayout, setPetOnlyLayout, setSidePanelLayout, PET_WITH_SIDE_W, PET_WITH_SIDE_H } from '@/services/windowLayout'
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
let unlistenProactive: (() => void) | null = null

const isBrowserDev = computed(() => !isTauri())
const isChatWindow = computed(() => winLabel.value === 'chat')
const isPetShell = computed(() => isBrowserDev.value || isPetWindowLabel(winLabel.value))

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
      handleProactiveMessage({ message: d.message, animation: d.animation })
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
      const history = await getChatHistory()
      rt.loadHistory(
        (history ?? []).map((m: { role: string; content: string }) => ({
          role: m.role as 'user' | 'assistant',
          content: m.content,
        })),
      )
    } catch (e) {
      console.warn('[load] chat history optional, skipped', e)
    }

    setupWs()
    void rt.ensurePushConnected()
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
    if (auth.isLoggedIn) void loadUserData()
    try {
      const { listen } = await import('@tauri-apps/api/event')
      await listen('chat-opened', async (event) => {
        auth.syncFromStorage()
        const payload = event.payload as { token?: string | null } | undefined
        if (payload?.token && !auth.isLoggedIn) {
          auth.setToken(payload.token)
        }
        if (!auth.isLoggedIn) {
          console.warn('[chat] not logged in in chat window')
          return
        }
        await loadUserData()
      })
    } catch (e) {
      console.warn('[chat] init listener failed', e)
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
  void setLoginLayout()
}

async function applySidePanelLayout() {
  if (!isTauri() || isChatWindow.value) return
  if (showOnboarding.value || growth.showSettings || showAdopt.value) {
    await setSidePanelLayout()
  } else {
    await setPetOnlyLayout()
  }
}

watch(
  () => [showOnboarding.value, growth.showSettings, showAdopt.value] as const,
  () => {
    void applySidePanelLayout()
  },
)

onUnmounted(() => {
  healthMonitor.stop()
  unlistenProactive?.()
  unlistenProactive = null
})
</script>

<template>
  <div class="app-root">
    <!-- Vite browser dev -->
    <template v-if="isBrowserDev">
      <LoginView v-if="ready && !auth.isLoggedIn" @success="onLoginSuccess" />
      <template v-else-if="ready && auth.isLoggedIn">
        <div
          class="dual-shell"
          :class="{ 'dual-shell--expanded': showOnboarding || growth.showSettings || showAdopt }"
          :style="(showOnboarding || growth.showSettings || showAdopt) && isBrowserDev ? {
            width: PET_WITH_SIDE_W + 'px',
            height: PET_WITH_SIDE_H + 'px',
          } : undefined"
        >
          <PetView :side-panel-open="showOnboarding || growth.showSettings || showAdopt" />
          <AdoptView v-if="showAdopt" @adopted="onAdopted" @logout="onAdoptLogout" />
          <OnboardingView v-else-if="showOnboarding" @done="onOnboardingDone" />
          <SettingsPanel v-else-if="growth.showSettings" />
        </div>
        <p v-if="loadError && !showOnboarding && !showAdopt" class="load-error">{{ loadError }}</p>
      </template>
    </template>

    <!-- Tauri chat popup window (separate webview) -->
    <ChatPanel v-else-if="isChatWindow && ready" />

    <!-- Tauri pet window -->
    <template v-else-if="isPetShell && ready">
      <LoginView v-if="!auth.isLoggedIn" @success="onLoginSuccess" />
      <template v-else>
        <div
          class="dual-shell dual-shell--expanded"
          v-if="showOnboarding || growth.showSettings || showAdopt"
        >
          <PetView :side-panel-open="true" />
          <AdoptView v-if="showAdopt" @adopted="onAdopted" @logout="onAdoptLogout" />
          <OnboardingView v-else-if="showOnboarding" @done="onOnboardingDone" />
          <SettingsPanel v-else-if="growth.showSettings" />
        </div>
        <PetView v-else />
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

.dev-shell {
  display: flex;
  flex-direction: row;
  background: transparent;
}

.dual-shell {
  display: flex;
  flex-direction: row;
  align-items: stretch;
  width: 100%;
  height: 100%;
  background: transparent;
  overflow: hidden;
}

.dual-shell--expanded {
  width: 100%;
  height: 100%;
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
