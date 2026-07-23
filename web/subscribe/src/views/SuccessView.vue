<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { AdoptResult } from '../types'
import PetPreview from '../components/PetPreview.vue'

const router = useRouter()
const result = ref<AdoptResult | null>(null)

onMounted(() => {
  const raw = sessionStorage.getItem('mochi_last_adopt')
  if (!raw) {
    void router.replace({ name: 'catalog' })
    return
  }
  try {
    const parsed = JSON.parse(raw) as AdoptResult
    result.value = parsed
  } catch {
    void router.replace({ name: 'catalog' })
  }
})

function goCatalog() {
  void router.push({ name: 'catalog' })
}
</script>

<template>
  <div v-if="result" class="success card">
    <div class="celebrate">🎉</div>
    <h1>认购成功</h1>
    <p class="msg">{{ result.message || '欢迎新成员回家！' }}</p>

    <div class="pet-summary">
      <PetPreview :sku="result.sku" />
      <div>
        <h2>{{ result.pet.name || result.sku.name }}</h2>
        <p>{{ result.sku.breed_name }} · {{ result.sku.tagline }}</p>
      </div>
    </div>

    <section class="downloads">
      <h3>下一步：下载客户端</h3>
      <p class="dl-hint">用同一账号登录，Bond / 记忆 / 年龄跨端同步</p>
      <div class="dl-grid">
        <a class="dl-btn" href="#" @click.prevent>Windows 桌面版</a>
        <a class="dl-btn" href="#" @click.prevent>macOS 桌面版</a>
        <a class="dl-btn muted" href="#" @click.prevent>Android（即将上线）</a>
        <a class="dl-btn muted" href="#" @click.prevent>iOS（即将上线）</a>
      </div>
    </section>

    <button type="button" class="secondary" @click="goCatalog">再认养一只</button>
  </div>
</template>

<style scoped>
.card {
  background: white;
  border-radius: 20px;
  padding: 32px 24px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
  text-align: center;
}

.celebrate {
  font-size: 48px;
  margin-bottom: 8px;
}

h1 {
  margin: 0 0 8px;
  font-size: 24px;
}

.msg {
  color: #666;
  font-size: 14px;
  margin: 0 0 24px;
}

.pet-summary {
  display: flex;
  align-items: center;
  gap: 16px;
  justify-content: center;
  padding: 16px;
  background: #fafafa;
  border-radius: 16px;
  margin-bottom: 24px;
  text-align: left;
}

.pet-summary h2 {
  margin: 0 0 4px;
  font-size: 18px;
}

.pet-summary p {
  margin: 0;
  font-size: 13px;
  color: #666;
}

.downloads {
  text-align: left;
  margin-bottom: 20px;
}

.downloads h3 {
  margin: 0 0 6px;
  font-size: 15px;
}

.dl-hint {
  margin: 0 0 12px;
  font-size: 12px;
  color: #999;
}

.dl-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.dl-btn {
  display: block;
  padding: 12px;
  text-align: center;
  border-radius: 10px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  text-decoration: none;
  font-size: 13px;
  font-weight: 600;
}

.dl-btn.muted {
  background: #eee;
  color: #888;
  font-weight: 500;
}

.secondary {
  width: 100%;
  padding: 12px;
  border: 1px solid #eee;
  border-radius: 10px;
  background: white;
  color: #666;
  cursor: pointer;
  font-size: 14px;
}
</style>
