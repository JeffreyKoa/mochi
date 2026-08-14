<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  fetchPublicCapability,
  fetchPublicModules,
  markSetupWizardDone,
  MODULE_LABELS,
  statusLabel,
  updateSetupModules,
  type CapabilityReport,
  type ModuleName,
  type ModuleProvider,
  type ModuleView,
} from '@/services/moduleSetup'

const emit = defineEmits<{ done: [] }>()

const step = ref(0)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const capability = ref<CapabilityReport | null>(null)
const moduleViews = ref<ModuleView[]>([])

type ModuleDraft = {
  enabled: boolean
  provider: ModuleProvider
}

const draft = ref<Record<ModuleName, ModuleDraft>>({
  asr: { enabled: true, provider: 'auto' },
  tts: { enabled: true, provider: 'auto' },
  llm: { enabled: true, provider: 'remote' },
  vision: { enabled: true, provider: 'auto' },
  emotion: { enabled: true, provider: 'local' },
})

const apiKeyDraft = ref('')

const moduleNames: ModuleName[] = ['asr', 'tts', 'llm', 'vision', 'emotion']

const warnings = computed(() => {
  if (!capability.value) return []
  return capability.value.modules.filter(
    (m) => draft.value[m.name]?.enabled && m.status === 'unsupported',
  )
})

onMounted(async () => {
  loading.value = true
  error.value = ''
  try {
    const [cap, views] = await Promise.all([fetchPublicCapability(), fetchPublicModules()])
    capability.value = cap
    moduleViews.value = views
    for (const name of moduleNames) {
      const v = views.find((x) => x.name === name)
      if (v) {
        draft.value[name] = {
          enabled: v.enabled,
          provider: (v.provider as ModuleProvider) || 'auto',
        }
      }
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})

function nextStep() {
  if (step.value < 3) step.value += 1
}

function prevStep() {
  if (step.value > 0) step.value -= 1
}

async function finish() {
  saving.value = true
  error.value = ''
  try {
    const payload: Parameters<typeof updateSetupModules>[0] = {}
    for (const name of moduleNames) {
      payload[name] = { ...draft.value[name] }
    }
    if (apiKeyDraft.value.trim()) {
      payload.api_key = apiKeyDraft.value.trim()
    }
    await updateSetupModules(payload)
    markSetupWizardDone()
    emit('done')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function skipWizard() {
  markSetupWizardDone()
  emit('done')
}

function capRow(name: ModuleName) {
  return capability.value?.modules.find((m) => m.name === name)
}
</script>

<template>
  <div class="setup-overlay">
    <div class="setup-card">
      <header class="setup-header">
        <h2>Mochi 本地部署向导</h2>
        <p class="hint">选择需要的 AI 模块；关闭的模块不会下载模型或启动 sidecar。</p>
      </header>

      <p v-if="loading" class="hint">正在检测硬件与服务状态…</p>
      <p v-if="error" class="error">{{ error }}</p>

      <template v-if="!loading">
        <!-- Step 0: 模块勾选 -->
        <section v-show="step === 0" class="setup-body">
          <h3>1. 选择需要的模块</h3>
          <label v-for="name in moduleNames" :key="name" class="toggle-row">
            <span>{{ MODULE_LABELS[name] }}</span>
            <input v-model="draft[name].enabled" type="checkbox" />
          </label>
        </section>

        <!-- Step 1: 硬件报告 -->
        <section v-show="step === 1" class="setup-body">
          <h3>2. 硬件检测报告</h3>
          <p v-if="capability" class="hw-line">
            CPU {{ capability.hardware.cpu_cores }} 核 · 内存
            {{ capability.hardware.ram_total_mb }} MB
            <span v-if="capability.hardware.gpu.present">
              · GPU {{ capability.hardware.gpu.name }}
            </span>
          </p>
          <ul class="cap-list">
            <li v-for="name in moduleNames" :key="name">
              <strong>{{ MODULE_LABELS[name] }}</strong>
              <span v-if="!draft[name].enabled"> — 已关闭</span>
              <span v-else>
                — {{ statusLabel(capRow(name)?.status ?? 'ok') }}
                <em v-if="capRow(name)?.reason">（{{ capRow(name)?.reason }}）</em>
              </span>
            </li>
          </ul>
          <p v-for="w in warnings" :key="w.name" class="warn">
            {{ MODULE_LABELS[w.name] }}：硬件不足，建议改「远程 API」或关闭此模块。
          </p>
        </section>

        <!-- Step 2: 运行方式 -->
        <section v-show="step === 2" class="setup-body">
          <h3>3. 各模块运行方式</h3>
          <div v-for="name in moduleNames" :key="name" class="provider-row">
            <span>{{ MODULE_LABELS[name] }}</span>
            <select v-model="draft[name].provider" :disabled="!draft[name].enabled">
              <option value="auto">自动（本地优先）</option>
              <option value="local">仅本地</option>
              <option value="remote">仅远程 API</option>
            </select>
          </div>
        </section>

        <!-- Step 3: API Key -->
        <section v-show="step === 3" class="setup-body">
          <h3>4. 远程 API（可选）</h3>
          <p class="hint">
            LLM 或远程 ASR/TTS/视觉需要 API Key。也可稍后在 config/config.yaml 中填写 ai.api_key。
          </p>
          <input
            v-model="apiKeyDraft"
            class="text-input"
            type="password"
            placeholder="sk-...（留空则跳过）"
            autocomplete="off"
          />
          <p class="hint">保存后请运行 scripts/restart-backend.ps1 使 sidecar 变更生效。</p>
        </section>
      </template>

      <footer class="setup-footer">
        <button type="button" class="ghost" @click="skipWizard">跳过</button>
        <div class="spacer" />
        <button v-if="step > 0" type="button" class="ghost" @click="prevStep">上一步</button>
        <button v-if="step < 3" type="button" class="primary-sm" :disabled="loading" @click="nextStep">
          下一步
        </button>
        <button
          v-else
          type="button"
          class="primary-sm"
          :disabled="loading || saving"
          @click="finish"
        >
          {{ saving ? '保存中…' : '完成' }}
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.setup-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}
.setup-card {
  width: min(480px, 100%);
  max-height: 90vh;
  overflow: auto;
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
}
.setup-header h2 {
  margin: 0 0 8px;
  font-size: 1.15rem;
}
.setup-body h3 {
  margin: 0 0 12px;
  font-size: 1rem;
}
.toggle-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid #eee;
}
.cap-list {
  margin: 0;
  padding-left: 1.2rem;
  font-size: 0.9rem;
}
.hw-line {
  font-size: 0.85rem;
  color: #555;
}
.provider-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.provider-row select {
  flex: 1;
  max-width: 180px;
}
.text-input {
  width: 100%;
  padding: 8px;
  border: 1px solid #ccc;
  border-radius: 8px;
}
.setup-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
}
.spacer {
  flex: 1;
}
.hint {
  color: #666;
  font-size: 0.85rem;
}
.error {
  color: #c0392b;
  font-size: 0.85rem;
}
.warn {
  color: #d35400;
  font-size: 0.85rem;
}
.ghost {
  background: transparent;
  border: none;
  color: #666;
  cursor: pointer;
}
.primary-sm {
  padding: 8px 16px;
  border-radius: 8px;
  border: none;
  background: #6c5ce7;
  color: #fff;
  cursor: pointer;
}
</style>
