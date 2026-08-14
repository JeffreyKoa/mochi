import { getApiBase } from '@/config'

/** 模块名称（与后端 capability API 一致） */
export type ModuleName = 'asr' | 'tts' | 'llm' | 'vision' | 'emotion'

export type ModuleProvider = 'local' | 'remote' | 'auto'

export type CapabilityStatus =
  | 'disabled'
  | 'ok'
  | 'degraded'
  | 'unsupported'
  | 'remote_ready'
  | 'remote_missing'

export interface ModulePublicEntry {
  enabled: boolean
  provider: string
}

export interface ModulesPublic {
  asr: ModulePublicEntry
  tts: ModulePublicEntry
  llm: ModulePublicEntry
  vision: ModulePublicEntry
  emotion: ModulePublicEntry
}

export interface CapabilityModuleReport {
  name: ModuleName
  enabled: boolean
  provider: string
  status: CapabilityStatus
  reason?: string
  requires_local_sidecar?: boolean
}

export interface CapabilityReport {
  hardware: {
    cpu_cores: number
    cpu_model?: string
    ram_total_mb: number
    ram_available_mb: number
    disk_free_mb: number
    gpu: { present: boolean; name?: string; vram_total_mb?: number }
  }
  modules: CapabilityModuleReport[]
}

export interface ModuleView {
  name: ModuleName
  enabled: boolean
  provider: string
  capability_status: CapabilityStatus
  capability_reason?: string
  sidecar_healthy?: boolean
  model_ready?: boolean
  requires_local_sidecar: boolean
}

const SETUP_WIZARD_KEY = 'mochi_setup_wizard_v1'
const CAPABILITY_CACHE_KEY = 'mochi_capability_cache_v1'

let _capabilityCache: CapabilityReport | null = null

export function getCachedCapability(): CapabilityReport | null {
  if (_capabilityCache) return _capabilityCache
  if (typeof window === 'undefined') return null
  try {
    const raw = localStorage.getItem(CAPABILITY_CACHE_KEY)
    if (!raw) return null
    _capabilityCache = JSON.parse(raw) as CapabilityReport
    return _capabilityCache
  } catch {
    return null
  }
}

function cacheCapability(report: CapabilityReport) {
  _capabilityCache = report
  try {
    localStorage.setItem(CAPABILITY_CACHE_KEY, JSON.stringify(report))
  } catch {
    /* ignore quota */
  }
}

export async function fetchPublicCapability(): Promise<CapabilityReport> {
  const res = await fetch(`${getApiBase()}/api/v1/public/capability`, {
    signal: AbortSignal.timeout(15000),
  })
  if (!res.ok) throw new Error(`capability http ${res.status}`)
  const data = (await res.json()) as CapabilityReport
  cacheCapability(data)
  return data
}

export async function fetchPublicModules(): Promise<ModuleView[]> {
  const res = await fetch(`${getApiBase()}/api/v1/public/modules`, {
    signal: AbortSignal.timeout(15000),
  })
  if (!res.ok) throw new Error(`modules http ${res.status}`)
  const data = (await res.json()) as { modules: ModuleView[] }
  return data.modules ?? []
}

export interface ModulePatch {
  enabled?: boolean
  provider?: ModuleProvider
}

export interface UpdateModulesPayload {
  asr?: ModulePatch
  tts?: ModulePatch
  llm?: ModulePatch
  vision?: ModulePatch
  emotion?: ModulePatch
  api_key?: string
}

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('mochi_token')
  return {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  }
}

/** 更新服务端 config.yaml modules（需登录）。 */
export async function updateSetupModules(payload: UpdateModulesPayload): Promise<void> {
  const res = await fetch(`${getApiBase()}/api/v1/setup/modules`, {
    method: 'PUT',
    headers: authHeaders(),
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(20000),
  })
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(err.error ?? `setup modules http ${res.status}`)
  }
}

export function isSetupWizardDone(): boolean {
  if (typeof window === 'undefined') return true
  return localStorage.getItem(SETUP_WIZARD_KEY) === '1'
}

export function markSetupWizardDone(): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(SETUP_WIZARD_KEY, '1')
}

export function shouldShowSetupWizard(): boolean {
  return !isSetupWizardDone()
}

export const MODULE_LABELS: Record<ModuleName, string> = {
  asr: '语音识别 (ASR)',
  tts: '语音合成 (TTS)',
  llm: '对话大模型 (LLM)',
  vision: '视觉感知',
  emotion: '声学情感',
}

export function statusLabel(status: CapabilityStatus): string {
  switch (status) {
    case 'disabled':
      return '已关闭'
    case 'ok':
      return '就绪'
    case 'degraded':
      return '可用（降级）'
    case 'unsupported':
      return '硬件不足'
    case 'remote_ready':
      return '远程 API'
    case 'remote_missing':
      return '需配置 API Key'
    default:
      return status
  }
}
