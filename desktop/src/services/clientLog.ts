/**
 * 桌宠/聊天前端日志落盘（Tauri）。
 * 开发：仓库 logs/desktop/desktop-YYYYMMDD.log
 * 安装版：%LOCALAPPDATA%/Mochi/logs/desktop/desktop-YYYYMMDD.log
 */
import { getCurrentWindow } from '@tauri-apps/api/window'
import { isTauri, isTauriInvokeReady } from '@/services/chatWindow'

type ConsoleFn = (...args: unknown[]) => void

let windowLabel = 'webview'
let queue: string[] = []
let flushTimer: ReturnType<typeof setTimeout> | null = null
let installed = false
let flushing = false
let flushErrorLogged = false

async function waitInvokeReady(maxMs = 8000): Promise<boolean> {
  const start = Date.now()
  while (Date.now() - start < maxMs) {
    if (isTauriInvokeReady()) return true
    await new Promise((r) => setTimeout(r, 50))
  }
  return false
}

const orig: Record<'log' | 'info' | 'warn' | 'error' | 'debug', ConsoleFn> = {
  log: console.log.bind(console),
  info: console.info.bind(console),
  warn: console.warn.bind(console),
  error: console.error.bind(console),
  debug: console.debug.bind(console),
}

/** 与 Go log 对齐的时间前缀：2026/08/08 18:35:00 */
function formatTimestamp(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}/${p(d.getMonth() + 1)}/${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

function serializeArg(value: unknown): string {
  if (value instanceof Error) {
    return value.stack || value.message
  }
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function interpolatePrintf(template: string, args: unknown[]): string {
  if (!args.length || (!template.includes('%s') && !template.includes('%d') && !template.includes('%i') && !template.includes('%f'))) {
    return template
  }
  let argIdx = 0
  return template.replace(/%[sdif]/g, () => {
    if (argIdx >= args.length) return '%?'
    const v = args[argIdx++]
    if (typeof v === 'number') return Number.isInteger(v) ? String(v) : v.toFixed(3)
    return serializeArg(v)
  })
}

function formatLine(level: string, args: unknown[]): string {
  if (args.length === 0) return `${formatTimestamp()} [${windowLabel}][${level}]`
  const first = args[0]
  if (typeof first === 'string' && args.length > 1 && /%[sdif]/.test(first)) {
    const body = interpolatePrintf(first, args.slice(1))
    return `${formatTimestamp()} [${windowLabel}][${level}] ${body}`
  }
  const body = args.map(serializeArg).join(' ')
  return `${formatTimestamp()} [${windowLabel}][${level}] ${body}`
}

function enqueue(level: string, args: unknown[]) {
  if (!installed || !isTauri()) return
  queue.push(formatLine(level, args))
  if (queue.length >= 40) {
    void flushClientLogQueue()
    return
  }
  if (!flushTimer) {
    flushTimer = setTimeout(() => {
      flushTimer = null
      void flushClientLogQueue()
    }, 120)
  }
}

async function flushClientLogQueue() {
  if (!isTauri() || flushing || queue.length === 0) return
  flushing = true
  const batch = queue.splice(0, 80)
  try {
    const { invoke } = await import('@tauri-apps/api/core')
    await invoke('append_client_logs', { lines: batch })
  } catch (e) {
    if (!flushErrorLogged) {
      flushErrorLogged = true
      orig.error('[client-log] append_client_logs failed (check Tauri permissions):', e)
    }
  } finally {
    flushing = false
    if (queue.length > 0) {
      void flushClientLogQueue()
    }
  }
}

function wrapConsole(level: keyof typeof orig, tag: string) {
  return (...args: unknown[]) => {
    orig[level](...args)
    enqueue(tag, args)
  }
}

/** 在 createApp 之前调用，尽早捕获启动期日志。 */
export async function initClientLog(): Promise<void> {
  if (!isTauri() || installed) return

  const invokeReady = await waitInvokeReady()
  if (!invokeReady) {
    orig.warn('[client-log] Tauri invoke not ready; file sink disabled')
    return
  }

  installed = true

  try {
    windowLabel = getCurrentWindow().label
  } catch {
    windowLabel = 'unknown'
  }

  console.log = wrapConsole('log', 'INFO')
  console.info = wrapConsole('info', 'INFO')
  console.warn = wrapConsole('warn', 'WARN')
  console.error = wrapConsole('error', 'ERROR')
  console.debug = wrapConsole('debug', 'DEBUG')

  window.addEventListener('error', (ev) => {
    enqueue('ERROR', [
      `uncaught: ${ev.message}`,
      ev.filename ? `${ev.filename}:${ev.lineno}:${ev.colno}` : '',
    ])
  })
  window.addEventListener('unhandledrejection', (ev) => {
    enqueue('ERROR', ['unhandledrejection', ev.reason])
  })

  window.addEventListener('beforeunload', () => {
    void flushClientLogQueue()
  })

  enqueue('INFO', [`client log sink ready label=${windowLabel}`])
  void flushClientLogQueue()
}

/** 打开客户端日志目录（资源管理器）。 */
export async function openClientLogDir(): Promise<void> {
  if (!isTauri()) return
  const { invoke } = await import('@tauri-apps/api/core')
  await invoke('open_client_logs')
}

export async function getClientLogDir(): Promise<string> {
  if (!isTauri()) return ''
  const { invoke } = await import('@tauri-apps/api/core')
  return invoke<string>('get_client_log_dir')
}
