const TOKEN_KEY = 'mochi_web_token'
const EMAIL_KEY = 'mochi_web_email'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setAuth(token: string, email: string) {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(EMAIL_KEY, email)
}

export function getEmail(): string {
  return localStorage.getItem(EMAIL_KEY) ?? ''
}

export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(EMAIL_KEY)
}

export function isLoggedIn(): boolean {
  return !!getToken()
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(init?.headers ?? {}),
  }
  const res = await fetch(path, { ...init, headers })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || `请求失败 (${res.status})`)
  }
  return data as T
}

export async function login(email: string, password: string) {
  return request<{ token: string }>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function register(email: string, password: string) {
  return request<{ token: string }>('/api/v1/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function fetchCatalog() {
  const data = await request<{ skus: import('./types').PetSKU[] }>('/api/v1/catalog/skus')
  return data.skus ?? []
}

export async function adoptPet(skuId: string, petName?: string) {
  return request<import('./types').AdoptResult>('/api/v1/subscribe/adopt', {
    method: 'POST',
    body: JSON.stringify({ sku_id: skuId, pet_name: petName }),
  })
}
