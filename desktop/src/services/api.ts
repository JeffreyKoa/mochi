import { getApiBase, initClientConfig, setApiBase, getClientConfig } from '@/config'

export type ApiErrorKind = 'network' | 'server' | 'client'

export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
}

export class ApiError extends Error {
  kind: ApiErrorKind
  status?: number

  constructor(kind: ApiErrorKind, message: string, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.kind = kind
    this.status = status
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

const REQUEST_TIMEOUT_MS = 20000

async function request<T = Record<string, unknown>>(url: string, init?: RequestInit): Promise<{ res: Response; data: T }> {
  let res: Response
  try {
    res = await fetch(url, { ...init, signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'TimeoutError') {
      throw new ApiError('network', '连接后端超时，Mochi 在等你…')
    }
    throw new ApiError('network', '无法连接后端，Mochi 在等你…')
  }

  const text = await res.text()
  let data: T
  if (text) {
    try {
      data = JSON.parse(text) as T
    } catch {
      const hint = res.status === 404 ? '接口不存在，请确认后端已更新并重启' : `服务器响应格式错误 (${res.status})`
      throw new ApiError('client', hint, res.status)
    }
  } else {
    data = {} as T
  }

  const errMsg = (data as { error?: string }).error

  if (res.status === 401) {
    throw new AuthError(errMsg || '登录已过期，请重新登录')
  }
  if (res.status === 503) {
    throw new ApiError('server', errMsg || '后端繁忙，请稍后再试', res.status)
  }
  if (res.status >= 500) {
    throw new ApiError('server', errMsg || '后端暂时不可用，请稍后再试', res.status)
  }
  if (!res.ok) {
    throw new ApiError('client', errMsg || `请求失败 (${res.status})`, res.status)
  }

  return { res, data }
}

export async function register(email: string, password: string, petName?: string) {
  const { data } = await request(`${getApiBase()}/api/v1/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, pet_name: petName }),
  })
  return data
}

export async function login(email: string, password: string) {
  const { data } = await request(`${getApiBase()}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  return data
}

export async function getPet() {
  const { data } = await request(`${getApiBase()}/api/v1/pet`, { headers: authHeaders() })
  return data
}

export async function getCatalogSKUs() {
  const { data } = await request<{ skus?: unknown[] }>(`${getApiBase()}/api/v1/catalog/skus`)
  return data.skus ?? []
}

export async function adoptPet(skuId: string, petName?: string) {
  const { data } = await request(`${getApiBase()}/api/v1/subscribe/adopt`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ sku_id: skuId, pet_name: petName }),
  })
  return data
}

export async function getLifeState() {
  const { data } = await request(`${getApiBase()}/api/v1/life/state`, { headers: authHeaders() })
  return data
}

export async function interact(type: 'touch' | 'feed' | 'play') {
  const { data } = await request(`${getApiBase()}/api/v1/life/interact`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ type }),
  })
  return data
}

export async function getChatHistory(limit = 50) {
  const { data } = await request<{ messages?: unknown[] }>(
    `${getApiBase()}/api/v1/chat/history?limit=${limit}`,
    { headers: authHeaders() },
  )
  return data.messages
}

export async function getMemories() {
  const { data } = await request<{ memories?: unknown[] }>(`${getApiBase()}/api/v1/memories`, {
    headers: authHeaders(),
  })
  return data.memories
}

export async function deleteMemory(id: number) {
  await request(`${getApiBase()}/api/v1/memories/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
}

export async function getBond() {
  const { data } = await request(`${getApiBase()}/api/v1/bond`, { headers: authHeaders() })
  return data
}

export async function getBrief() {
  const { data } = await request<{
    brief?: unknown
    entries?: unknown[]
    pending_entries?: unknown[]
    write_approval?: boolean
  }>(`${getApiBase()}/api/v1/brief`, { headers: authHeaders() })
  return data
}

export async function approveBriefEntry(id: number) {
  const { data } = await request(`${getApiBase()}/api/v1/brief/entries/${id}/approve`, {
    method: 'POST',
    headers: authHeaders(),
  })
  return data
}

export async function rejectBriefEntry(id: number) {
  const { data } = await request(`${getApiBase()}/api/v1/brief/entries/${id}/reject`, {
    method: 'POST',
    headers: authHeaders(),
  })
  return data
}

export async function getUserPreferences() {
  const { data } = await request<{ proactive_enabled?: boolean }>(
    `${getApiBase()}/api/v1/user/preferences`,
    { headers: authHeaders() },
  )
  return data
}

export async function updateUserPreferences(body: { proactive_enabled: boolean }) {
  const { data } = await request(`${getApiBase()}/api/v1/user/preferences`, {
    method: 'PUT',
    headers: authHeaders(),
    body: JSON.stringify(body),
  })
  return data
}

export async function completeOnboarding(body: {
  user_calls_pet?: string
  pet_calls_user?: string
  traits?: string
  speech_style?: string
  first_topic?: string
  first_joke?: string
}) {
  const { data } = await request(`${getApiBase()}/api/v1/pet/onboarding`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(body),
  })
  return data
}

export async function updatePetName(name: string) {
  const { data } = await request(`${getApiBase()}/api/v1/pet/name`, {
    method: 'PUT',
    headers: authHeaders(),
    body: JSON.stringify({ name }),
  })
  return data
}

export interface ReminderItem {
  id: number
  title: string
  fire_at: string
  status: string
}

export interface TodoItem {
  id: number
  title: string
  due_at?: string
  done: boolean
}

export async function getReminders(status = 'pending') {
  const { data } = await request<{ reminders?: ReminderItem[] }>(
    `${getApiBase()}/api/v1/reminders?status=${encodeURIComponent(status)}`,
    { headers: authHeaders() },
  )
  return data.reminders ?? []
}

export async function cancelReminder(id: number) {
  const { data } = await request(`${getApiBase()}/api/v1/reminders/${id}`, {
    method: 'PATCH',
    headers: authHeaders(),
    body: JSON.stringify({ status: 'cancelled' }),
  })
  return data
}

export async function getTodos(done = false) {
  const { data } = await request<{ todos?: TodoItem[] }>(
    `${getApiBase()}/api/v1/todos?done=${done}`,
    { headers: authHeaders() },
  )
  return data.todos ?? []
}

export async function completeTodo(id: number) {
  const { data } = await request(`${getApiBase()}/api/v1/todos/${id}`, {
    method: 'PATCH',
    headers: authHeaders(),
    body: JSON.stringify({ done: true }),
  })
  return data
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

export { getApiBase, getClientConfig, getToken, initClientConfig, setApiBase }
