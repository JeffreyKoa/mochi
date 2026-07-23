<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { adoptPet, getCatalogSKUs } from '@/services/api'
import { useAuthStore } from '@/stores/authStore'
import type { PetSKU } from '@/types/petSkin'

const emit = defineEmits<{ adopted: []; logout: [] }>()
const auth = useAuthStore()

const skus = ref<PetSKU[]>([])
const loading = ref(true)
const adopting = ref('')
const error = ref('')

onMounted(async () => {
  try {
    skus.value = (await getCatalogSKUs()) as PetSKU[]
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载图鉴失败'
  } finally {
    loading.value = false
  }
})

async function pick(sku: PetSKU) {
  if (adopting.value) return
  adopting.value = sku.sku_id
  error.value = ''
  try {
    const pendingName = sessionStorage.getItem('mochi_pending_pet_name') || undefined
    await adoptPet(sku.sku_id, pendingName)
    sessionStorage.removeItem('mochi_pending_pet_name')
    emit('adopted')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '认购失败'
  } finally {
    adopting.value = ''
  }
}

function previewStyle(sku: PetSKU) {
  const c = sku.skin?.colors
  if (!c) return {}
  return {
    background: `linear-gradient(160deg, ${c.idle}, ${c.happy})`,
  }
}
function logout() {
  auth.logout()
  emit('logout')
}
</script>

<template>
  <div class="adopt-root">
    <div class="adopt-panel">
      <div class="adopt-header">
        <span>选择你的跟屁虫</span>
        <button type="button" class="logout-btn" @click="logout">退出登录</button>
      </div>

      <div class="adopt-body">
        <p v-if="auth.email" class="account">当前账号：{{ auth.email }}</p>
        <p class="hint">认购流程（支付已跳过）· 选一只带回家</p>

        <div v-if="loading" class="loading">加载图鉴…</div>

        <ul v-else class="sku-list">
          <li v-for="sku in skus" :key="sku.sku_id" class="sku-card">
            <div class="preview" :style="previewStyle(sku)">
              <span class="ear left" :style="{ background: sku.skin?.colors?.ear_inner }" />
              <span class="ear right" :style="{ background: sku.skin?.colors?.ear_inner }" />
              <span class="face" />
            </div>
            <div class="info">
              <h3>{{ sku.name }}</h3>
              <p class="tagline">{{ sku.tagline }}</p>
              <p class="meta">{{ sku.breed_name }} · 约 {{ sku.max_age_years }} 年 · ¥{{ sku.price_cny }}</p>
              <button
                type="button"
                class="adopt-btn"
                :disabled="!!adopting"
                @click="pick(sku)"
              >
                {{ adopting === sku.sku_id ? '认领中…' : '免费认领' }}
              </button>
            </div>
          </li>
        </ul>

        <p v-if="error" class="error">{{ error }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.adopt-root {
  width: 320px;
  height: 440px;
  flex-shrink: 0;
  background: #fff;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.18);
}

.adopt-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.adopt-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-weight: 600;
  font-size: 14px;
}

.logout-btn {
  border: none;
  background: rgba(255, 255, 255, 0.2);
  color: white;
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 500;
  flex-shrink: 0;
}

.logout-btn:hover {
  background: rgba(255, 255, 255, 0.35);
}

.account {
  margin: 0 0 6px;
  font-size: 11px;
  color: #888;
  text-align: center;
}

.adopt-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px;
}

.hint {
  margin: 0 0 12px;
  font-size: 12px;
  color: #999;
  text-align: center;
}

.loading {
  text-align: center;
  color: #888;
  padding: 24px;
  font-size: 12px;
}

.sku-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.sku-card {
  display: flex;
  gap: 12px;
  padding: 10px;
  border-radius: 12px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
}

.preview {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  flex-shrink: 0;
  position: relative;
}

.ear {
  position: absolute;
  width: 18px;
  height: 28px;
  border-radius: 50% 50% 40% 40%;
  top: 4px;
}

.ear.left {
  left: 8px;
  transform: rotate(-18deg);
}

.ear.right {
  right: 8px;
  transform: rotate(18deg);
}

.face {
  position: absolute;
  left: 50%;
  top: 52%;
  transform: translate(-50%, -50%);
  width: 24px;
  height: 4px;
  background: rgba(51, 51, 51, 0.35);
  border-radius: 2px;
}

.info {
  flex: 1;
  min-width: 0;
}

.info h3 {
  margin: 0 0 4px;
  font-size: 14px;
  color: #333;
}

.tagline {
  margin: 0 0 4px;
  font-size: 12px;
  color: #666;
  line-height: 1.4;
}

.meta {
  margin: 0 0 8px;
  font-size: 11px;
  color: #aaa;
}

.adopt-btn {
  width: 100%;
  padding: 8px;
  border: none;
  border-radius: 10px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.adopt-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.error {
  margin-top: 10px;
  font-size: 12px;
  color: #e74c3c;
  text-align: center;
}
</style>
