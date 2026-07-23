<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="app-shell">
    <header class="site-header">
      <router-link to="/catalog" class="brand">🍡 Mochi 认购</router-link>
      <span class="tagline">选一只跟屁虫，带回家</span>
    </header>
    <main>
      <RouterView />
    </main>
    <footer class="site-footer">Mochi · 情感伙伴 · 支付演示已跳过</footer>
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.site-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 16px 24px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
}

.brand {
  font-size: 18px;
  font-weight: 700;
  color: white;
  text-decoration: none;
}

.tagline {
  font-size: 13px;
  opacity: 0.9;
}

main {
  flex: 1;
  padding: 24px 16px 32px;
  max-width: 720px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}

.site-footer {
  text-align: center;
  padding: 16px;
  font-size: 12px;
  color: #aaa;
}
</style>
