<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRealtimeStore } from '@/stores/realtimeStore'
import {
  getEventLoopMaxLagMs,
  getEventLoopLagMs,
  EVENT_LOOP_LAG_WARN_MS,
} from '@/services/eventLoopProbe'
import {
  getVoiceSidecarStatus,
  type VoiceSidecarStatus,
} from '@/services/voiceSidecar'
import { getBaselineSampleCount } from '@/services/turnMetricsBaseline'
import { isTauri } from '@/services/chatWindow'

const rt = useRealtimeStore()
const collapsed = ref(false)
const eventLoopLagMs = ref(0)
const eventLoopMaxLagMs = ref(0)
let raf = 0
let probeTimer: ReturnType<typeof setInterval> | null = null

function refreshLag() {
  eventLoopLagMs.value = getEventLoopLagMs()
  eventLoopMaxLagMs.value = getEventLoopMaxLagMs()
  raf = requestAnimationFrame(refreshLag)
}

const lagClass = computed(() =>
  eventLoopLagMs.value >= EVENT_LOOP_LAG_WARN_MS ? 'warn' : '',
)

const sidecarStatus = ref<VoiceSidecarStatus | null>(null)

const sidecarManagedLabel = computed(() => {
  if (!isTauri()) return 'browser'
  const s = sidecarStatus.value
  if (!s) return '…'
  return s.bundleMode === 'release' ? `managed (${s.xasr.state}/${s.xtts.state})` : `dev (${s.xasr.state}/${s.xtts.state})`
})

const xasrSidecarLabel = computed(() => {
  const v = rt.xasrSidecarReachable
  if (v === null) return '…'
  if (v && rt.sttBackendLabel === 'xasr') return 'online (session)'
  return v ? 'online' : 'offline'
})

const xasrSidecarClass = computed(() => {
  if (rt.xasrSidecarReachable === false) return 'warn'
  if (rt.xasrSidecarReachable === true) return 'ok'
  return ''
})

const metricsLines = computed(() => {
  const m = rt.lastTurnMetrics
  if (!m) return ['(no turn yet)']
  const fmt = (v: number) => (v >= 0 ? `${v}ms` : '—')
  return [
    `audioEnd ${fmt(m.audioEndMs)}`,
    `asrFinal ${fmt(m.asrFinalMs)}`,
    `vision ${fmt(m.visionMs)}`,
    `parallel ${fmt(m.perceiveParallelMs)}`,
    `llm1st ${fmt(m.llmFirstTokenMs)}`,
    `tts1st ${fmt(m.ttsFirstByteMs)}`,
    `playback ${fmt(m.playbackStartMs)}`,
  ]
})

onMounted(() => {
  raf = requestAnimationFrame(refreshLag)
  void rt.refreshXasrSidecarProbe()
  void rt.refreshXttsSidecarProbe()
  if (isTauri()) {
    void getVoiceSidecarStatus().then((s) => { sidecarStatus.value = s })
  }
  probeTimer = setInterval(() => {
    void rt.refreshXasrSidecarProbe()
    void rt.refreshXttsSidecarProbe()
    if (isTauri()) {
      void getVoiceSidecarStatus().then((s) => { sidecarStatus.value = s })
    }
  }, 15000)
})

onUnmounted(() => {
  cancelAnimationFrame(raf)
  if (probeTimer) clearInterval(probeTimer)
})
</script>

<template>
  <div class="voice-diag" :class="{ collapsed }">
    <button type="button" class="voice-diag__toggle" @click="collapsed = !collapsed">
      {{ collapsed ? '▸ RT' : '▾ RT diag' }}
    </button>
    <div v-if="!collapsed" class="voice-diag__body">
      <div class="row">
        <span class="k">turn</span>
        <span class="v">{{ rt.turnPhase }}</span>
      </div>
      <div class="row">
        <span class="k">perception</span>
        <span class="v">{{ rt.perceptionPhase }}</span>
      </div>
      <div class="row">
        <span class="k">talking</span>
        <span class="v">{{ rt.talking }} / rest {{ rt.resting }}</span>
      </div>
      <div class="row">
        <span class="k">sidecar</span>
        <span class="v">{{ sidecarManagedLabel }}</span>
      </div>
      <div class="row" :class="xasrSidecarClass">
        <span class="k">xasrSidecar</span>
        <span class="v">{{ xasrSidecarLabel }}</span>
      </div>
      <div class="row">
        <span class="k">stt</span>
        <span class="v">{{ rt.sttBackendLabel || (rt.talking ? '…' : '—') }}</span>
      </div>
      <div class="row">
        <span class="k">tts</span>
        <span class="v">{{ rt.ttsBackendLabel || (rt.talking ? '…' : '—') }}</span>
      </div>
      <div class="row">
        <span class="k">xttsSidecar</span>
        <span class="v">{{ rt.xttsSidecarReachable === null ? '…' : rt.xttsSidecarReachable ? 'online' : 'offline' }}</span>
      </div>
      <div class="row">
        <span class="k">chunks</span>
        <span class="v">{{ rt.chunksSent }}</span>
      </div>
      <div class="row">
        <span class="k">lastSpeech</span>
        <span class="v">
          {{ rt.lastSpeechAtMs ? `${Date.now() - rt.lastSpeechAtMs}ms ago` : '—' }}
        </span>
      </div>
      <div class="row" :class="lagClass">
        <span class="k">eventLoop</span>
        <span class="v">{{ eventLoopLagMs }}ms (max {{ eventLoopMaxLagMs }})</span>
      </div>
      <div class="row">
        <span class="k">baseline</span>
        <span class="v">n={{ getBaselineSampleCount() }}</span>
      </div>
      <div class="metrics">
        <div v-for="(line, i) in metricsLines" :key="i">{{ line }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.voice-diag {
  position: fixed;
  top: 4px;
  right: 4px;
  z-index: 9999;
  font-family: ui-monospace, monospace;
  font-size: 10px;
  line-height: 1.35;
  color: #e8e8e8;
  background: rgba(20, 22, 28, 0.88);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 6px;
  padding: 4px 6px;
  max-width: 220px;
  pointer-events: auto;
  user-select: text;
}

.voice-diag.collapsed {
  opacity: 0.75;
}

.voice-diag__toggle {
  all: unset;
  cursor: pointer;
  font-weight: 600;
  color: #8cf;
  display: block;
  width: 100%;
}

.voice-diag__body {
  margin-top: 4px;
}

.row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.row.warn .v {
  color: #ff6b6b;
  font-weight: 600;
}

.row.ok .v {
  color: #6bff8c;
}

.k {
  color: #888;
}

.metrics {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  color: #aaa;
}
</style>
