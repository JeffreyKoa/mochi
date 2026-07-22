import { getApiBase, initClientConfig, setApiBase } from '@/config'

export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
}

function getToken(): string | null {
  return localStorage.getItem('mochi_token')
}

function authHeaders(): HeadersInit {
  const token = getToken()
  return {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  }
}

async function parseResponse<T = Record<string, unknown>>(res: Response): Promise<T> {
  const text = await res.text()
  if (!text) {
    if (res.status === 401) throw new AuthError('登录已过期，请重新登录')
    if (!res.ok) throw new Error(`服务器无响应 (${res.status})，请确认后端已启动`)
    throw new Error('服务器返回空响应，请稍后重试')
  }
  try {
    return JSON.parse(text) as T
  } catch {
    throw new Error('服务器响应格式错误')
  }
}

async function request<T = Record<string, unknown>>(url: string, init?: RequestInit): Promise<{ res: Response; data: T }> {
  const res = await fetch(url, init)
  const data = await parseResponse<T>(res)
  return { res, data }
}

export async function register(email: string, password: string, petName?: string) {
  const { res, data } = await request(`${getApiBase()}/api/v1/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, pet_name: petName }),
  })
  if (!res.ok) throw new Error((data as { error?: string }).error || 'register failed')
  return data
}

export async function login(email: string, password: string) {
  const { res, data } = await request(`${getApiBase()}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) throw new Error((data as { error?: string }).error || 'login failed')
  return data
}

export async function getPet() {
  const { res, data } = await request(`${getApiBase()}/api/v1/pet`, { headers: authHeaders() })
  if (res.status === 401) throw new AuthError((data as { error?: string }).error || '登录已过期')
  if (!res.ok) throw new Error((data as { error?: string }).error || 'get pet failed')
  return data
}

export async function getLifeState() {
  const { res, data } = await request(`${getApiBase()}/api/v1/life/state`, { headers: authHeaders() })
  if (res.status === 401) throw new AuthError((data as { error?: string }).error || '登录已过期')
  if (!res.ok) throw new Error((data as { error?: string }).error || 'get state failed')
  return data
}

export async function interact(type: 'touch' | 'feed' | 'play') {
  const { res, data } = await request(`${getApiBase()}/api/v1/life/interact`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ type }),
  })
  if (res.status === 401) throw new AuthError((data as { error?: string }).error || '登录已过期')
  if (!res.ok) throw new Error((data as { error?: string }).error || 'interact failed')
  return data
}

export async function getChatHistory(limit = 50) {
  const { res, data } = await request<{ messages?: unknown[]; error?: string }>(
    `${getApiBase()}/api/v1/chat/history?limit=${limit}`,
    { headers: authHeaders() },
  )
  if (res.status === 401) throw new AuthError(data.error || '登录已过期')
  if (!res.ok) throw new Error(data.error || 'get history failed')
  return data.messages
}

export async function getMemories() {
  const { res, data } = await request<{ memories?: unknown[]; error?: string }>(
    `${getApiBase()}/api/v1/memories`,
    { headers: authHeaders() },
  )
  if (res.status === 401) throw new AuthError(data.error || '登录已过期')
  if (!res.ok) throw new Error(data.error || 'get memories failed')
  return data.memories
}

export async function deleteMemory(id: number) {
  const { res, data } = await request(`${getApiBase()}/api/v1/memories/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
  if (res.status === 401) throw new AuthError((data as { error?: string }).error || '登录已过期')
  if (!res.ok) throw new Error((data as { error?: string }).error || 'delete failed')
}

export function getWSUrl(): string {
  const token = getToken()
  const base = getApiBase()
  if (!base) {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}/ws?token=${token}`
  }
  const wsBase = base.replace(/^http/, 'ws')
  return `${wsBase}/ws?token=${token}`
}

export { getApiBase, getToken, initClientConfig, setApiBase }
