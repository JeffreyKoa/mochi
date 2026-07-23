<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { adoptPet, clearAuth, fetchCatalog, getEmail } from '../api'
import type { PetSKU } from '../types'
import PetPreview from '../components/PetPreview.vue'

const router = useRouter()
const skus = ref<PetSKU[]>([])
const loading = ref(true)
const adopting = ref('')
const error = ref('')
const petName = ref('Mochi')
const email = ref(getEmail())

onMounted(async () => {
  try {
    skus.value = await fetchCatalog()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})

async function adopt(sku: PetSKU) {
  if (adopting.value) return
  adopting.value = sku.sku_id
  error.value = ''
  try {
    const result = await adoptPet(sku.sku_id, petName.value.trim() || undefined)
    sessionStorage.setItem(
      'mochi_last_adopt',
      JSON.stringify({ pet: result.pet, sku: result.sku, message: result.message }),
    )
    await router.push({ name: 'success' })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '认领失败'
  } finally {
    adopting.value = ''
  }
}

function logout() {
  clearAuth()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="catalog">
    <div class="toolbar">
      <div>
        <h1>选择你的跟屁虫</h1>
        <p class="hint">认购流程（支付已跳过）· 选一只带回家</p>
      </div>
      <button type="button" class="ghost" @click="logout">退出 {{ email ? `(${email})` : '' }}</button>
    </div>

    <label class="name-row">
      <span>给它起名（可选）</span>
      <input v-model="petName" type="text" maxlength="32" placeholder="Mochi" />
    </label>

    <p v-if="loading" class="status">加载图鉴…</p>

    <ul v-else class="sku-list">
      <li v-for="sku in skus" :key="sku.sku_id" class="sku-card">
        <PetPreview :sku="sku" />
        <div class="info">
          <h2>{{ sku.name }}</h2>
          <p class="tagline">{{ sku.tagline }}</p>
          <p class="meta">
            {{ sku.breed_name }} · 约 {{ sku.max_age_years }} 年 ·
            <s v-if="sku.price_cny">¥{{ sku.price_cny }}</s>
            <strong> 免费</strong>
          </p>
          <button
            type="button"
            class="adopt-btn"
            :disabled="!!adopting"
            @click="adopt(sku)"
          >
            {{ adopting === sku.sku_id ? '认领中…' : '免费认领' }}
          </button>
        </div>
      </li>
    </ul>

    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>

<style scoped>
.catalog {
  width: 100%;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
}

h1 {
  margin: 0 0 4px;
  font-size: 22px;
}

.hint {
  margin: 0;
  font-size: 13px;
  color: #888;
}

.ghost {
  border: 1px solid #eee;
  background: white;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12px;
  color: #666;
  cursor: pointer;
  flex-shrink: 0;
}

.name-row {
  display: block;
  margin-bottom: 20px;
}

.name-row span {
  display: block;
  font-size: 12px;
  color: #666;
  margin-bottom: 6px;
}

.name-row input {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #e8e8e8;
  border-radius: 10px;
  font-size: 14px;
}

.status {
  text-align: center;
  color: #888;
  padding: 40px;
}

.sku-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.sku-card {
  display: flex;
  gap: 16px;
  padding: 16px;
  background: white;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.06);
}

.info {
  flex: 1;
  min-width: 0;
}

.info h2 {
  margin: 0 0 6px;
  font-size: 17px;
}

.tagline {
  margin: 0 0 6px;
  font-size: 13px;
  color: #555;
  line-height: 1.45;
}

.meta {
  margin: 0 0 12px;
  font-size: 12px;
  color: #999;
}

.meta s {
  opacity: 0.6;
}

.meta strong {
  color: #e05;
}

.adopt-btn {
  width: 100%;
  padding: 10px;
  border: none;
  border-radius: 10px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
}

.adopt-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.error {
  margin-top: 16px;
  text-align: center;
  color: #e74c3c;
  font-size: 13px;
}
</style>
