import { defineStore } from 'pinia'
import { ref } from 'vue'
import { login, register } from '@/services/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('mochi_token'))
  const email = ref(localStorage.getItem('mochi_email') ?? '')
  const isLoggedIn = ref(!!token.value)
  const error = ref('')

  function setToken(t: string) {
    token.value = t
    localStorage.setItem('mochi_token', t)
    isLoggedIn.value = true
  }

  function logout() {
    token.value = null
    email.value = ''
    localStorage.removeItem('mochi_token')
    localStorage.removeItem('mochi_email')
    isLoggedIn.value = false
  }

  /** Sync login state from localStorage (needed for separate Tauri webview windows). */
  function syncFromStorage() {
    const t = localStorage.getItem('mochi_token')
    if (t) {
      if (token.value !== t) setToken(t)
    } else if (token.value) {
      logout()
    }
  }

  async function doLogin(e: string, password: string) {
    error.value = ''
    const data = await login(e, password)
    setToken(data.token)
    email.value = e
    localStorage.setItem('mochi_email', e)
    return data
  }

  async function doRegister(e: string, password: string, petName?: string) {
    error.value = ''
    const data = await register(e, password, petName)
    setToken(data.token)
    email.value = e
    localStorage.setItem('mochi_email', e)
    return data
  }

  return { token, email, isLoggedIn, error, setToken, logout, syncFromStorage, doLogin, doRegister }
})
