<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { useAuthStore } from '@/stores/authStore'
import { usePetStore } from '@/stores/petStore'
import { useRealtimeStore } from '@/stores/realtimeStore'
import { getPet, getLifeState, getChatHistory, initClientConfig, AuthError, ApiError } from '@/services/api'
import { healthMonitor } from '@/services/healthMonitor'
import { wsManager } from '@/services/ws'
import {
  ensurePetWindowVisible,
  initPetWindowChrome,
  isPetWindowLabel,
  isTauri,
} from '@/services/chatWindow'
import { setLoginLayout, setPetOnlyLayout } from '@/services/windowLayout'
import LoginView from '@/views/LoginView.vue'
import PetView from '@/views/PetView.vue'
import ChatPanel from '@/components/chat/ChatPanel.vue'

const auth = useAuthStore()
const pet = usePetStore()
const rt = useRealtimeStore()
const ready = ref(false)
const loading = ref(true)
const loadError = ref('')
const winLabel = ref('browser')
const wsInitialized = ref(false)

const isBrowserDev = computed(() => !isTauri())
const isChatWindow = computed(() => winLabel.value === 'chat')
const isPetShell = computed(() => isBrowserDev.value || isPetWindowLabel(winLabel.value))

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
      if (d.animation) pet.setAnimation(d.animation as typeof pet.currentAnimation)
      pet.showSpeechBubble(d.message)
    })
  }
  wsManager.connect()
}

function startHealthWatch() {
  if (healthMonitor.watching) return
  healthMonitor.start(
    () => {
      loadError.value = ''
      pet.showSpeechBubble('连上了~')
      void loadUserData()
    },
    (_attempt, up) => {
      if (!up && !loadError.value.includes('重试')) {
        loadError.value = '无法连接后端，Mochi 在等你…'
      }
    },
  )
}

function handleLoadFailure(e: unknown) {
  if (e instanceof AuthError) {
    healthMonitor.stop()
    auth.logout()
    return
  }
  loadError.value = e instanceof Error ? e.message : '加载数据失败'
  if (e instanceof ApiError && (e.kind === 'network' || e.kind === 'server')) {
    startHealthWatch()
  }
}

async function loadUserData() {
  loadError.value = ''
  try {
    const petData = (await getPet()) as {
      name: string
      life_state?: Parameters<typeof pet.updateLifeState>[0]
    }
    pet.petName = petData.name
    if (petData.life_state) {
      pet.updateLifeState(petData.life_state)
      pet.syncAnimationFromState()
    } else {
      const state = await getLifeState()
      pet.updateLifeState(state)
      pet.syncAnimationFromState()
    }

    const history = await getChatHistory()
    rt.loadHistory(
      (history ?? []).map((m: { role: string; content: string }) => ({
        role: m.role as 'user' | 'assistant',
        content: m.content,
      })),
    )

    setupWs()
    healthMonitor.stop()
    loadError.value = ''
  } catch (e) {
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

  if (isChatWindow.value) {
    auth.syncFromStorage()
    await initClientConfig().catch((e) => console.warn('[chat] config', e))
    loading.value = false
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

onUnmounted(() => {
  healthMonitor.stop()
})
</script>

<template>
  <div class="app-root">
    <!-- Vite browser dev -->
    <template v-if="isBrowserDev">
      <LoginView v-if="ready && !auth.isLoggedIn" @success="onLoginSuccess" />
      <template v-else-if="ready && auth.isLoggedIn">
        <p v-if="loadError" class="load-error">{{ loadError }}</p>
        <div class="dev-shell">
          <PetView />
        </div>
      </template>
    </template>

    <!-- Tauri chat popup window (separate webview) -->
    <ChatPanel v-else-if="isChatWindow && ready" />

    <!-- Tauri pet window -->
    <template v-else-if="isPetShell && ready">
      <LoginView v-if="!auth.isLoggedIn" @success="onLoginSuccess" />
      <PetView v-else />
      <p v-if="auth.isLoggedIn && loading" class="boot-hint">Mochi 醒来中...</p>
      <p v-if="loadError" class="load-error">{{ loadError }}</p>
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
