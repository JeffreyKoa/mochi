let _apiBase = ''

export interface ClientConfig {
  apiBase: string
  realtimeEnabled: boolean
  writeApproval: boolean
  growthEnabled: boolean
}

let _clientConfig: ClientConfig = {
  apiBase: '',
  realtimeEnabled: true,
  writeApproval: false,
  growthEnabled: true,
}

export function getApiBase(): string {
  return _apiBase
}

export function getClientConfig(): ClientConfig {
  return _clientConfig
}

export function setApiBase(url: string) {
  _apiBase = url.replace(/\/$/, '')
  _clientConfig = { ..._clientConfig, apiBase: _apiBase }
}

function applyPublicConfig(base: string, data: Record<string, unknown>) {
  if (typeof data.api_base === 'string' && data.api_base) {
    setApiBase(data.api_base)
  } else {
    setApiBase(base)
  }
  _clientConfig = {
    apiBase: _apiBase,
    realtimeEnabled: data.realtime_enabled !== false,
    writeApproval: !!data.write_approval,
    growthEnabled: data.growth_enabled !== false,
  }
  return _clientConfig
}

/** 初始化 API 地址：Vite 开发走代理（相对路径），Tauri/生产读 config.yaml */
export async function initClientConfig(): Promise<ClientConfig> {
  if (typeof window !== 'undefined' && window.location.port === '1420') {
    setApiBase('')
    try {
      const res = await fetch('/api/v1/public/config')
      if (res.ok) {
        return applyPublicConfig('', (await res.json()) as Record<string, unknown>)
      }
    } catch {
      // proxy may be down; keep defaults
    }
    _clientConfig = { ..._clientConfig, apiBase: '' }
    return _clientConfig
  }

  const fallbacks = ['http://localhost:8081', 'http://localhost:8080']
  for (const base of fallbacks) {
    try {
      const res = await fetch(`${base}/api/v1/public/config`)
      if (res.ok) {
        return applyPublicConfig(base, (await res.json()) as Record<string, unknown>)
      }
    } catch {
      // try next
    }
  }

  setApiBase('http://localhost:8081')
  return _clientConfig
}
