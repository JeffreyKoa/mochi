<script setup lang="ts">
/** 设置页统一卡片：一卡一主题，避免 Tab 内混谈。 */
defineProps<{
  title?: string
  hint?: string
  badge?: string
  variant?: 'default' | 'danger'
}>()
</script>

<template>
  <section class="settings-card" :class="variant">
    <div v-if="title || badge" class="settings-card-head">
      <h3 class="settings-card-title">{{ title }}</h3>
      <span v-if="badge" class="settings-card-badge">{{ badge }}</span>
    </div>
    <p v-if="hint" class="settings-card-hint">{{ hint }}</p>
    <div class="settings-card-body">
      <slot />
    </div>
  </section>
</template>
