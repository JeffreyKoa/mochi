import { invoke } from '@tauri-apps/api/core'
import { postActivityHeartbeat } from './api'
import { isTauri } from './chatWindow'

const HEARTBEAT_MS = 5 * 60 * 1000

interface ActivitySnapshot {
  idle_seconds: number
  continuous_active_minutes: number
  session_active_minutes_today: number
  active_app: string
}

let timer: ReturnType<typeof setInterval> | null = null
let running = false

async function readSnapshot(): Promise<ActivitySnapshot | null> {
  if (!isTauri()) return null
  try {
    return await invoke<ActivitySnapshot>('get_activity_snapshot')
  } catch (e) {
    console.warn('[activity] snapshot failed', e)
    return null
  }
}

async function sendHeartbeat() {
  const snap = await readSnapshot()
  if (!snap) return
  try {
    await postActivityHeartbeat(snap)
  } catch (e) {
    console.warn('[activity] heartbeat failed', e)
  }
}

export function startActivityHeartbeat() {
  if (running || !isTauri()) return
  running = true
  void sendHeartbeat()
  timer = setInterval(() => void sendHeartbeat(), HEARTBEAT_MS)
}

export function stopActivityHeartbeat() {
  running = false
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

export async function getLocalActivitySnapshot(): Promise<ActivitySnapshot | null> {
  return readSnapshot()
}
