<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/authStore'

const emit = defineEmits<{ success: [] }>()
const auth = useAuthStore()

const mode = ref<'login' | 'register'>('login')
const email = ref('')
const password = ref('')
const petName = ref('Mochi')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    if (mode.value === 'login') {
      await auth.doLogin(email.value, password.value)
    } else {
      sessionStorage.setItem('mochi_pending_pet_name', petName.value.trim() || 'Mochi')
      await auth.doRegister(email.value, password.value, petName.value)
    }
    emit('success')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-overlay">
    <div class="login-card">
      <h1>🍡 Mochi</h1>
      <p class="subtitle">你的 AI 生命伙伴</p>

      <div class="tabs">
        <button :class="{ active: mode === 'login' }" @click="mode = 'login'">登录</button>
        <button :class="{ active: mode === 'register' }" @click="mode = 'register'">注册</button>
      </div>

      <form @submit.prevent="submit">
        <input v-model="email" type="email" placeholder="邮箱" required />
        <input v-model="password" type="password" placeholder="密码 (至少6位)" required minlength="6" />
        <input
          v-if="mode === 'register'"
          v-model="petName"
          type="text"
          placeholder="宠物名字"
          maxlength="32"
        />
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" :disabled="loading">
          {{ loading ? '请稍候...' : mode === 'login' ? '登录' : '创建 Mochi' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login-overlay {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(20, 20, 30, 0.85);
  backdrop-filter: blur(12px);
}

.login-card {
  width: 320px;
  padding: 32px 28px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.95);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

h1 {
  text-align: center;
  font-size: 28px;
  color: #333;
}

.subtitle {
  text-align: center;
  color: #888;
  margin: 8px 0 24px;
  font-size: 14px;
}

.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
}

.tabs button {
  flex: 1;
  padding: 8px;
  border: none;
  border-radius: 8px;
  background: #f0f0f0;
  cursor: pointer;
  font-size: 14px;
}

.tabs button.active {
  background: #ff8fab;
  color: white;
}

form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

input {
  padding: 12px 14px;
  border: 1px solid #e0e0e0;
  border-radius: 10px;
  font-size: 14px;
  outline: none;
}

input:focus {
  border-color: #ff8fab;
}

button[type='submit'] {
  padding: 12px;
  border: none;
  border-radius: 10px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  margin-top: 4px;
}

button[type='submit']:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.error {
  color: #e74c3c;
  font-size: 13px;
  text-align: center;
}
</style>
