/**
 * 在场主动闲聊：休息态周期性确认主人在镜头前 → 请求服务端找话题搭话。
 */
import { getCompanionPresenceConfig, getFaceprintConfig, initClientConfig } from '@/config'
import { postPresenceChat, getFaceprintStatus, getUserPreferences } from '@/services/api'
import {
  FaceVerifier,
  FACE_OWNER_BOOST_SCORE,
  MIN_FACE_DETECT_SCORE,
} from '@/services/faceVerifier'

/** 在场闲聊专用人脸校验实例（与 realtimeStore 内实例独立，避免循环依赖） */
const faceVerifier = new FaceVerifier()
import {
  captureOwnerFaceJPEG,
  isVisionCaptureEnabled,
  prewarmVisionSession,
  visionSession,
} from '@/services/visionCapture'
import { isLowPowerMode } from '@/services/lowPowerMode'
import { isTauri } from '@/services/chatWindow'
import type { SoundPresenceState } from '@/services/soundPresence'

const LOCAL_COOLDOWN_KEY = 'mochi_presence_chat_last_at'
const LOCAL_DAILY_KEY = 'mochi_presence_chat_daily'

export type PresenceChatPayload = {
  message: string
  animation?: string
}

type PhaseGuard = () => boolean

let timer: ReturnType<typeof setInterval> | null = null
let running = false
let consecutiveOwnerHits = 0
let phaseGuard: PhaseGuard = () => true
let deliverHandler: ((payload: PresenceChatPayload) => void) | null = null
let prefsEnabled = true
let serverEnabled = true
let triggerInFlight = false
let ownerFaceEmbedding: Float32Array | null = null

/** 注册阶段守卫：仅 resting/idle 时探测。 */
export function registerPresenceChatPhaseGuard(fn: PhaseGuard) {
  phaseGuard = fn
}

/** 注册送达回调（由 App.vue 绑定 realtimeStore）。 */
export function registerPresenceChatDeliver(handler: (payload: PresenceChatPayload) => void) {
  deliverHandler = handler
}

/** ambientMic 调用：vision 在场闲聊开启时跳过「你回来了~」固定气泡。 */
export function shouldSuppressAudioReturnBubble(): boolean {
  return prefsEnabled && serverEnabled && isVisionCaptureEnabled()
}

function todayKey(): string {
  return new Date().toISOString().slice(0, 10)
}

function localOnCooldown(cooldownMin: number): boolean {
  const raw = localStorage.getItem(LOCAL_COOLDOWN_KEY)
  if (!raw) return false
  const last = Number(raw)
  if (!Number.isFinite(last)) return false
  return Date.now() - last < cooldownMin * 60 * 1000
}

function localDailyExceeded(max: number): boolean {
  const key = `${LOCAL_DAILY_KEY}:${todayKey()}`
  const count = Number(localStorage.getItem(key) ?? '0')
  return count >= max
}

function markLocalSent(cooldownMin: number) {
  localStorage.setItem(LOCAL_COOLDOWN_KEY, String(Date.now()))
  const key = `${LOCAL_DAILY_KEY}:${todayKey()}`
  const count = Number(localStorage.getItem(key) ?? '0')
  localStorage.setItem(key, String(count + 1))
}

function ownerFaceConfirmed(face: { match: boolean; score: number; detected: boolean } | null): boolean {
  if (!face?.detected || face.score < MIN_FACE_DETECT_SCORE) return false
  if (face.match) return true
  return face.score >= FACE_OWNER_BOOST_SCORE
}

async function loadOwnerFaceprint() {
  ownerFaceEmbedding = null
  try {
    const status = await getFaceprintStatus()
    if (status.enrolled && status.embedding?.length) {
      ownerFaceEmbedding = new Float32Array(status.embedding)
    }
  } catch {
    ownerFaceEmbedding = null
  }
}

async function probeOwnerFace(): Promise<{ ok: boolean; score: number }> {
  const fp = getFaceprintConfig()
  if (!fp.enabled) return { ok: false, score: 0 }
  if (!ownerFaceEmbedding?.length) {
    await loadOwnerFaceprint()
  }
  if (!ownerFaceEmbedding?.length) return { ok: false, score: 0 }

  await faceVerifier.init().catch(() => {})
  if (!faceVerifier.available) return { ok: false, score: 0 }

  const buf = visionSession.isActive()
    ? await visionSession.grabSnapshot()
    : await captureOwnerFaceJPEG()
  if (!buf) return { ok: false, score: 0 }

  try {
    const result = await faceVerifier.verify(buf, ownerFaceEmbedding, fp.matchThreshold)
    if (!result) return { ok: false, score: 0 }
    return { ok: ownerFaceConfirmed(result), score: result.score }
  } catch {
    return { ok: false, score: 0 }
  }
}

async function tryTriggerVision() {
  if (triggerInFlight || !running) return
  if (!phaseGuard()) {
    consecutiveOwnerHits = 0
    return
  }
  if (!prefsEnabled || !serverEnabled || !isVisionCaptureEnabled()) return
  if (isLowPowerMode()) return

  const cfg = getCompanionPresenceConfig()
  if (localOnCooldown(cfg.cooldownMin) || localDailyExceeded(cfg.dailyMax)) return

  const probe = await probeOwnerFace()
  if (!probe.ok) {
    consecutiveOwnerHits = 0
    return
  }
  consecutiveOwnerHits++
  if (consecutiveOwnerHits < 2) return

  consecutiveOwnerHits = 0
  triggerInFlight = true
  try {
    const res = await postPresenceChat({
      face_match: true,
      face_score: probe.score,
      trigger: 'vision',
    })
    if (res.ok && res.message) {
      markLocalSent(cfg.cooldownMin)
      deliverHandler?.({ message: res.message, animation: res.animation })
    }
  } catch (e) {
    console.warn('[presence_chat] trigger failed', e)
  } finally {
    triggerInFlight = false
  }
}

/** 声音感知降级：主人从 away 回到 owner_present 时触发。 */
export async function tryTriggerAudioPresence(prev: SoundPresenceState, next: SoundPresenceState) {
  if (!running || triggerInFlight) return
  if (!(prev === 'away' && next === 'owner_present')) return
  if (!phaseGuard() || !prefsEnabled || !serverEnabled) return

  // vision + 脸纹可用时优先视觉链路，避免双通道重复
  if (isVisionCaptureEnabled() && getFaceprintConfig().enabled) {
    try {
      const st = await getFaceprintStatus()
      if (st.enrolled) return
    } catch {
      // 继续 audio 降级
    }
  }

  const cfg = getCompanionPresenceConfig()
  if (localOnCooldown(cfg.cooldownMin) || localDailyExceeded(cfg.dailyMax)) return

  triggerInFlight = true
  try {
    const res = await postPresenceChat({ trigger: 'audio', face_match: false, face_score: 0 })
    if (res.ok && res.message) {
      markLocalSent(cfg.cooldownMin)
      deliverHandler?.({ message: res.message, animation: res.animation })
    }
  } catch (e) {
    console.warn('[presence_chat] audio trigger failed', e)
  } finally {
    triggerInFlight = false
  }
}

async function refreshPrefs() {
  try {
    const prefs = await getUserPreferences()
    prefsEnabled = prefs.proactive_enabled !== false && prefs.presence_chat_enabled !== false
  } catch {
    prefsEnabled = true
  }
  await initClientConfig().catch(() => {})
  serverEnabled = getCompanionPresenceConfig().enabled
}

export async function startPresenceChatLoop() {
  if (running || !isTauri()) return
  running = true
  await refreshPrefs()
  await loadOwnerFaceprint()
  void prewarmVisionSession()
  const cfg = getCompanionPresenceConfig()
  const intervalMs = Math.max(15_000, cfg.intervalSec * 1000)
  timer = setInterval(() => void tryTriggerVision(), intervalMs)
  setTimeout(() => void tryTriggerVision(), 12_000)
}

export function stopPresenceChatLoop() {
  running = false
  consecutiveOwnerHits = 0
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

export function refreshPresenceChatPrefs() {
  void refreshPrefs()
}
