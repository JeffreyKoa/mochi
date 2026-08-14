<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  fetchPublicCapability,
  fetchPublicModules,
  MODULE_LABELS,
  statusLabel,
  updateSetupModules,
  type ModuleName,
  type ModuleProvider,
  type ModuleView,
} from '@/services/moduleSetup'
import SettingsCard from '@/components/settings/SettingsCard.vue'

const moduleNames: ModuleName[] = ['asr', 'tts', 'llm', 'vision', 'emotion']

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const success = ref('')
const views = ref<ModuleView[]>([])

const draft = ref<Record<ModuleName, { enabled: boolean; provider: ModuleProvider }>>({
  asr: { enabled: true, provider: 'auto' },
  tts: { enabled: true, provider: 'auto' },
  llm: { enabled: true, provider: 'remote' },
  vision: { enabled: true, provider: 'auto' },
  emotion: { enabled: true, provider: 'local' },
})

onMounted(() => {
  void reload()
})

async function reload() {
  loading.value = true
  error.value = ''
  try {
    await fetchPublicCapability()
    const list = await fetchPublicModules()
    views.value = list
    for (const name of moduleNames) {
      const v = list.find((x) => x.name === name)
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
}

function viewOf(name: ModuleName) {
  return views.value.find((v) => v.name === name)
}

async function saveModule(name: ModuleName) {
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    await updateSetupModules({ [name]: { ...draft.value[name] } })
    success.value = '已保存。请运行 scripts/restart-backend.ps1 重启 sidecar。'
    await reload()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <SettingsCard title="AI 模块" hint="开关控制是否加载本地模型；auto=本地优先，失败时远程 API">
    <p v-if="loading" class="hint">加载模块状态…</p>
    <div v-for="name in moduleNames" :key="name" class="mod-row">
      <div class="mod-head">
        <label class="toggle-inline">
          <input v-model="draft[name].enabled" type="checkbox" />
          <span>{{ MODULE_LABELS[name] }}</span>
        </label>
        <span class="status-badge">{{ statusLabel(viewOf(name)?.capability_status ?? 'ok') }}</span>
      </div>
      <select v-model="draft[name].provider" class="mod-select" :disabled="!draft[name].enabled">
        <option value="auto">自动</option>
        <option value="local">本地</option>
        <option value="remote">远程</option>
      </select>
      <p v-if="viewOf(name)?.capability_reason" class="hint tiny">
        {{ viewOf(name)?.capability_reason }}
      </p>
      <button type="button" class="primary-sm" :disabled="saving" @click="saveModule(name)">
        应用
      </button>
    </div>
    <p v-if="success" class="ok">{{ success }}</p>
    <p v-if="error" class="error">{{ error }}</p>
  </SettingsCard>
</template>

<style scoped>
.mod-row {
  border-bottom: 1px solid #eee;
  padding: 10px 0;
}
.mod-row:last-child {
  border-bottom: none;
}
.mod-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.toggle-inline {
  display: flex;
  align-items: center;
  gap: 8px;
}
.status-badge {
  font-size: 0.75rem;
  color: #666;
  background: #f0f0f0;
  padding: 2px 8px;
  border-radius: 999px;
}
.mod-select {
  width: 100%;
  margin-bottom: 6px;
  padding: 6px;
  border-radius: 8px;
  border: 1px solid #ddd;
}
.hint.tiny {
  font-size: 0.8rem;
  margin: 0 0 6px;
}
.ok {
  color: #27ae60;
  font-size: 0.85rem;
}
.error {
  color: #c0392b;
  font-size: 0.85rem;
}
.primary-sm {
  padding: 6px 12px;
  border-radius: 8px;
  border: none;
  background: #6c5ce7;
  color: #fff;
  cursor: pointer;
  font-size: 0.85rem;
}
</style>
