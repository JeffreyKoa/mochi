<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login, register, setAuth } from '../api'

const router = useRouter()
const mode = ref<'login' | 'register'>('login')
const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    const data =
      mode.value === 'login'
        ? await login(email.value, password.value)
        : await register(email.value, password.value)
    setAuth(data.token, email.value)
    await router.push({ name: 'catalog' })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="card">
    <h1>欢迎来到 Mochi</h1>
    <p class="sub">登录或注册后，选择你的跟屁虫</p>

    <div class="tabs">
      <button type="button" :class="{ active: mode === 'login' }" @click="mode = 'login'">登录</button>
      <button type="button" :class="{ active: mode === 'register' }" @click="mode = 'register'">注册</button>
    </div>

    <form @submit.prevent="submit">
      <label>
        <span>邮箱</span>
        <input v-model="email" type="email" required placeholder="you@example.com" />
      </label>
      <label>
        <span>密码</span>
        <input v-model="password" type="password" required minlength="6" placeholder="至少 6 位" />
      </label>
      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" class="primary" :disabled="loading">
        {{ loading ? '请稍候…' : mode === 'login' ? '登录' : '注册并继续' }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.card {
  background: white;
  border-radius: 20px;
  padding: 32px 28px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
}

h1 {
  margin: 0 0 8px;
  font-size: 24px;
  text-align: center;
}

.sub {
  margin: 0 0 24px;
  text-align: center;
  color: #888;
  font-size: 14px;
}

.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
}

.tabs button {
  flex: 1;
  padding: 10px;
  border: none;
  border-radius: 10px;
  background: #f3f3f3;
  cursor: pointer;
  font-size: 14px;
}

.tabs button.active {
  background: #ff8fab;
  color: white;
  font-weight: 600;
}

form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

label span {
  display: block;
  font-size: 12px;
  color: #666;
  margin-bottom: 4px;
}

input {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid #e8e8e8;
  border-radius: 10px;
  font-size: 14px;
}

input:focus {
  outline: none;
  border-color: #ffb3c6;
}

.primary {
  margin-top: 8px;
  padding: 14px;
  border: none;
  border-radius: 12px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
}

.primary:disabled {
  opacity: 0.6;
  cursor: default;
}

.error {
  color: #e74c3c;
  font-size: 13px;
  text-align: center;
  margin: 0;
}
</style>
