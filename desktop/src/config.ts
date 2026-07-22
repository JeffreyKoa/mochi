let _apiBase = ''

export function getApiBase(): string {
  return _apiBase
}

export function setApiBase(url: string) {
  _apiBase = url.replace(/\/$/, '')
}

/** 初始化 API 地址：Vite 开发走代理（相对路径），Tauri/生产读 config.yaml */
export async function initClientConfig() {
  // 浏览器 + Vite dev：同源 + vite proxy，避免端口/CORS 问题
  if (typeof window !== 'undefined' && window.location.port === '1420') {
    setApiBase('')
    return ''
  }

  const fallbacks = ['http://localhost:8081', 'http://localhost:8080']
  for (const base of fallbacks) {
    try {
      const res = await fetch(`${base}/api/v1/public/config`)
      if (res.ok) {
        const data = await res.json()
        if (data.api_base) {
          setApiBase(data.api_base)
          return data.api_base as string
        }
      }
    } catch {
      // try next
    }
  }

  setApiBase('http://localhost:8081')
  return 'http://localhost:8081'
}
