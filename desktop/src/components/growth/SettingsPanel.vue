<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAuthStore } from '@/stores/authStore'
import { useGrowthStore } from '@/stores/growthStore'
import { usePetStore } from '@/stores/petStore'
import {
  updatePetName,
  getUserPreferences,
  updateUserPreferences,
  getLearningPreferences,
  updateLearningPreferences,
  getReminders,
  getTodos,
  cancelReminder,
  completeTodo,
  getVoiceprintStatus,
  enrollVoiceprint,
  deleteVoiceprint,
  getFaceprintStatus,
  enrollFaceprint,
  deleteFaceprint,
  type ReminderItem,
  type TodoItem,
  type LearningPreferences,
  type VoiceprintStatus,
  type FaceprintStatus,
} from '@/services/api'
import { PCMCapture } from '@/services/pcmCapture'
import { pcmToFloat } from '@/services/sileroSpeechVad'
import { SpeakerVerifier } from '@/services/speakerVerifier'
import {
  cacheVoiceprintEmbedding,
  clearVoiceprintEmbeddingCache,
} from '@/services/voiceprintCache'
import { FaceVerifier } from '@/services/faceVerifier'
import {
  CATEGORY_LABELS,
  parseInsideJokes,
  parseNicknames,
  parseSharedTopics,
} from '@/types/growth'
import {
  LIFE_AXIS,
  axisIndexForStage,
  companionHint,
  teachingHint,
  genderLabel,
  MOOD_LABELS,
} from '@/constants/lifecycle'
import { resolveVoiceLabel } from '@/constants/voiceProfile'
import { formatMemoryTime } from '@/utils/date'
import { getClientConfig, getFaceprintConfig, initClientConfig } from '@/config'
import {
  isVisionCaptureEnabled,
  setVisionCaptureEnabled,
  startVisionSession,
  stopVisionSession,
  visionSession,
} from '@/services/visionCapture'
import { hideSidePanelPopup, isTauri } from '@/services/chatWindow'
import { refreshPresenceChatPrefs } from '@/services/presenceChat'
import {
  micPermissionDeniedMessage,
  resetTauriMicrophonePermission,
} from '@/utils/micPermission'
import SettingsCard from '@/components/settings/SettingsCard.vue'

type TabId = 'mochi' | 'voice' | 'memory' | 'tasks' | 'me'

const growth = useGrowthStore()
const pet = usePetStore()
const auth = useAuthStore()
const tab = ref<TabId>('mochi')
const petNameDraft = ref('')
const savingName = ref(false)
const nameError = ref('')
const proactiveEnabled = ref(true)
const presenceChatEnabled = ref(true)
const morningGreeting = ref(true)
const followUpEnabled = ref(true)
const reminderVoice = ref(true)
const voiceReplyDefault = ref(true)
const sttMode = ref<'auto' | 'local' | 'cloud'>('auto')
const visionEnabled = ref(isVisionCaptureEnabled())
const serverVisionEnabled = ref(false)
const quietStart = ref(23)
const quietEnd = ref(8)
const wellnessEnabled = ref(true)
const wellnessDrink = ref(true)
const wellnessMeal = ref(true)
const wellnessRest = ref(true)
const lunchHour = ref(12)
const dinnerHour = ref(18)
const wellnessDailyMax = ref(2)
const savingProactive = ref(false)
const prefsError = ref('')
const reminders = ref<ReminderItem[]>([])
const todos = ref<TodoItem[]>([])
const doneReminders = ref<ReminderItem[]>([])
const doneTodos = ref<TodoItem[]>([])
const tasksLoading = ref(false)
const tasksError = ref('')
const learningTopics = ref<string[]>([])
const englishLevel = ref('daily')
const studyPace = ref(15)
const noUnsolicitedAdvice = ref(true)
const savingLearning = ref(false)
const learningError = ref('')
const showTaskHistory = ref(false)
const voiceAdvancedOpen = ref(false)

const voiceprintStatus = ref<VoiceprintStatus | null>(null)
const voiceprintLoading = ref(false)
const voiceprintError = ref('')
const faceprintStatus = ref<FaceprintStatus | null>(null)
const faceprintLoading = ref(false)
const faceprintError = ref('')
const enrollingFace = ref(false)
const faceEnrollProgress = ref('')
const micFixBusy = ref(false)
const micFixMsg = ref('')
const enrollingVoice = ref(false)
const enrollProgress = ref('')
const enrollCapture = new PCMCapture()
const enrollVerifier = new SpeakerVerifier()
const enrollFaceVerifier = new FaceVerifier()
const ENROLL_PROMPTS = [
  '你好 Mochi，我是你的主人。',
  '今天天气不错，我想和你聊聊天。',
  '记住我的声音，只回应我说的话。',
]
const ENROLL_MS = 3000

const nicknames = computed(() => parseNicknames(growth.bond?.nicknames))
const jokes = computed(() => parseInsideJokes(growth.bond?.inside_jokes))
const topics = computed(() => parseSharedTopics(growth.bond?.shared_topics))
const lifeAxisIndex = computed(() => axisIndexForStage(pet.lifecycle.life_stage))
const stageCompanionHint = computed(() => companionHint(pet.lifecycle.life_stage))
const stageTeachingHint = computed(() => teachingHint(pet.lifecycle.life_stage))
const voiceProfile = computed(() => resolveVoiceLabel(pet.gender, pet.lifecycle.life_stage))
const briefUsedChars = computed(() => growth.brief?.compiled_text?.length ?? 0)
const briefBudget = computed(() => growth.brief?.char_budget ?? 1400)
const lastMoodLabel = computed(() => {
  const tag = growth.bond?.last_mood_tag
  if (!tag || tag === 'neutral') return ''
  return MOOD_LABELS[tag] ?? tag
})

const tabs: { id: TabId; label: string }[] = [
  { id: 'mochi', label: 'Mochi' },
  { id: 'voice', label: '声音' },
  { id: 'memory', label: '记忆' },
  { id: 'tasks', label: '待办' },
  { id: 'me', label: '我的' },
]

const topicOptions = [
  { id: 'english', label: '英语' },
  { id: 'life', label: '为人处世' },
  { id: 'study_method', label: '学习方法' },
]

const englishLevels = [
  { id: 'beginner', label: '零基础' },
  { id: 'daily', label: '日常交流' },
  { id: 'work', label: '工作场景' },
]

function rapportLabel(level: number) {
  if (level >= 80) return '非常投缘'
  if (level >= 60) return '比较熟了'
  if (level >= 40) return '渐渐熟悉'
  return '互相了解中'
}

function trustLabel(level: number) {
  if (level >= 70) return '愿意说心里话'
  if (level >= 45) return '信任建立中'
  return '还在建立信任'
}

function formatTaskTime(iso: string) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

async function loadTasks() {
  tasksLoading.value = true
  tasksError.value = ''
  try {
    const [r, t, rf, td] = await Promise.all([
      getReminders('pending'),
      getTodos(false),
      getReminders('fired', 10),
      getTodos(true, 10),
    ])
    reminders.value = r
    todos.value = t
    doneReminders.value = rf
    doneTodos.value = td
  } catch (e) {
    tasksError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    tasksLoading.value = false
  }
}

function onVisionToggle() {
  visionEnabled.value = !visionEnabled.value
  setVisionCaptureEnabled(visionEnabled.value)
}

async function loadServerVisionFlag() {
  try {
    await initClientConfig()
    serverVisionEnabled.value = getClientConfig().visionEnabled
  } catch {
    serverVisionEnabled.value = false
  }
}

function syncReminderVoiceLocal() {
  localStorage.setItem('mochi_reminder_voice', reminderVoice.value ? '1' : '0')
}

function applyPreferences(prefs: Awaited<ReturnType<typeof getUserPreferences>>) {
  proactiveEnabled.value = prefs.proactive_enabled !== false
  presenceChatEnabled.value = prefs.presence_chat_enabled !== false
  quietStart.value = prefs.quiet_hours_start ?? 23
  quietEnd.value = prefs.quiet_hours_end ?? 8
  morningGreeting.value = prefs.morning_greeting !== false
  followUpEnabled.value = prefs.follow_up_enabled !== false
  reminderVoice.value = prefs.reminder_voice !== false
  voiceReplyDefault.value = prefs.voice_reply_default !== false
  sttMode.value = prefs.stt_mode ?? 'auto'
  wellnessEnabled.value = prefs.wellness_nudges_enabled !== false
  wellnessDrink.value = prefs.wellness_drink !== false
  wellnessMeal.value = prefs.wellness_meal !== false
  wellnessRest.value = prefs.wellness_rest !== false
  lunchHour.value = prefs.lunch_hour ?? 12
  dinnerHour.value = prefs.dinner_hour ?? 18
  wellnessDailyMax.value = prefs.wellness_daily_max ?? 2
  syncReminderVoiceLocal()
  localStorage.setItem('mochi_voice_reply_default', voiceReplyDefault.value ? '1' : '0')
  localStorage.setItem('mochi_stt_mode', sttMode.value)
}

async function loadPreferences() {
  try {
    const prefs = await getUserPreferences()
    applyPreferences(prefs)
  } catch {
    proactiveEnabled.value = true
  }
}

async function loadLearningPrefs() {
  try {
    const lp = await getLearningPreferences()
    learningTopics.value = lp.learning_topics ?? []
    englishLevel.value = lp.english_level || 'daily'
    studyPace.value = lp.study_pace_minutes || 15
    noUnsolicitedAdvice.value = lp.no_unsolicited_advice !== false
  } catch {
    learningTopics.value = []
  }
}

async function savePref(patch: Parameters<typeof updateUserPreferences>[0]) {
  savingProactive.value = true
  prefsError.value = ''
  try {
    const prefs = await updateUserPreferences(patch)
    applyPreferences(prefs)
  } catch (e) {
    prefsError.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    savingProactive.value = false
  }
}

async function onProactiveToggle() {
  await savePref({ proactive_enabled: !proactiveEnabled.value })
}

async function onPresenceChatToggle() {
  await savePref({ presence_chat_enabled: !presenceChatEnabled.value })
  refreshPresenceChatPrefs()
}

async function onMorningToggle() {
  await savePref({ morning_greeting: !morningGreeting.value })
}

async function onFollowUpToggle() {
  await savePref({ follow_up_enabled: !followUpEnabled.value })
}

async function onReminderVoiceToggle() {
  await savePref({ reminder_voice: !reminderVoice.value })
}

async function onVoiceReplyToggle() {
  await savePref({ voice_reply_default: !voiceReplyDefault.value })
}

async function onQuietChange() {
  await savePref({ quiet_hours_start: quietStart.value, quiet_hours_end: quietEnd.value })
}

async function onWellnessToggle() {
  await savePref({ wellness_nudges_enabled: !wellnessEnabled.value })
}

async function onWellnessDrinkToggle() {
  await savePref({ wellness_drink: !wellnessDrink.value })
}

async function onWellnessMealToggle() {
  await savePref({ wellness_meal: !wellnessMeal.value })
}

async function onWellnessRestToggle() {
  await savePref({ wellness_rest: !wellnessRest.value })
}

async function onMealTimeChange() {
  await savePref({ lunch_hour: lunchHour.value, dinner_hour: dinnerHour.value })
}

async function onWellnessMaxChange() {
  await savePref({ wellness_daily_max: wellnessDailyMax.value })
}

async function onSttModeChange() {
  await savePref({ stt_mode: sttMode.value })
}

function toggleLearningTopic(id: string) {
  const set = new Set(learningTopics.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  learningTopics.value = [...set]
}

async function saveLearningPrefs() {
  savingLearning.value = true
  learningError.value = ''
  const body: LearningPreferences = {
    learning_topics: learningTopics.value,
    english_level: englishLevel.value,
    study_pace_minutes: studyPace.value,
    no_unsolicited_advice: noUnsolicitedAdvice.value,
  }
  try {
    await updateLearningPreferences(body)
  } catch (e) {
    learningError.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    savingLearning.value = false
  }
}

async function onCancelReminder(id: number) {
  try {
    await cancelReminder(id)
    reminders.value = reminders.value.filter((x) => x.id !== id)
  } catch (e) {
    tasksError.value = e instanceof Error ? e.message : '取消失败'
  }
}

async function onCompleteTodo(id: number) {
  try {
    await completeTodo(id)
    todos.value = todos.value.filter((x) => x.id !== id)
  } catch (e) {
    tasksError.value = e instanceof Error ? e.message : '操作失败'
  }
}

function openTasksTab() {
  tab.value = 'tasks'
  void loadTasks()
}

function close() {
  if (isTauri()) {
    void (async () => {
      try {
        const { getCurrentWindow } = await import('@tauri-apps/api/window')
        if (getCurrentWindow().label === 'chat') {
          growth.closeSettings()
          await hideSidePanelPopup()
          const { emit } = await import('@tauri-apps/api/event')
          await emit('side-panel-closed', { mode: 'settings' })
          return
        }
      } catch {
        // fall through
      }
      growth.closeSettings()
    })()
    return
  }
  growth.closeSettings()
}

function openMochiTab() {
  tab.value = 'mochi'
  petNameDraft.value = pet.petName
}

function openVoiceTab() {
  tab.value = 'voice'
  void loadPreferences()
  void loadVoiceprintStatus()
  void loadFaceprintStatus()
}

function openMeTab() {
  tab.value = 'me'
  void loadPreferences()
  void loadLearningPrefs()
}

function selectTab(id: TabId) {
  if (id === 'mochi') openMochiTab()
  else if (id === 'me') openMeTab()
  else if (id === 'tasks') openTasksTab()
  else if (id === 'voice') openVoiceTab()
  else tab.value = id
}

async function loadVoiceprintStatus() {
  voiceprintLoading.value = true
  voiceprintError.value = ''
  try {
    voiceprintStatus.value = await getVoiceprintStatus()
    if (voiceprintStatus.value?.enrolled && voiceprintStatus.value.embedding?.length) {
      cacheVoiceprintEmbedding(voiceprintStatus.value.embedding)
    }
  } catch (e) {
    voiceprintStatus.value = null
    voiceprintError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    voiceprintLoading.value = false
  }
}

async function loadFaceprintStatus() {
  faceprintLoading.value = true
  faceprintError.value = ''
  try {
    faceprintStatus.value = await getFaceprintStatus()
  } catch (e) {
    faceprintStatus.value = null
    faceprintError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    faceprintLoading.value = false
  }
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms))
}

async function recordEnrollSegment(prompt: string): Promise<Float32Array | null> {
  enrollProgress.value = `请朗读：${prompt}`
  const chunks: Float32Array[] = []
  await enrollCapture.start((pcm) => {
    chunks.push(pcmToFloat(pcm))
  })
  await sleep(ENROLL_MS)
  await enrollCapture.stop()
  const total = chunks.reduce((n, c) => n + c.length, 0)
  if (total < 8000) return null
  const merged = new Float32Array(total)
  let off = 0
  for (const c of chunks) {
    merged.set(c, off)
    off += c.length
  }
  return merged
}

async function startVoiceprintEnroll() {
  if (enrollingVoice.value) return
  enrollingVoice.value = true
  voiceprintError.value = ''
  enrollProgress.value = '初始化声纹模型…'
  try {
    await enrollVerifier.init()
    if (!enrollVerifier.available) {
      voiceprintError.value =
        '声纹模型未就绪。请将 CAM++ ONNX 放到 public/models/speaker/campp.onnx'
      return
    }
    const embs: Float32Array[] = []
    for (let i = 0; i < ENROLL_PROMPTS.length; i++) {
      const pcm = await recordEnrollSegment(ENROLL_PROMPTS[i])
      if (!pcm) {
        voiceprintError.value = `第 ${i + 1} 段录音太短，请靠近麦克风再试`
        return
      }
      enrollProgress.value = `分析第 ${i + 1} 段…`
      const emb = await enrollVerifier.extract(pcm)
      if (!emb) {
        voiceprintError.value = `第 ${i + 1} 段分析失败，请重试`
        return
      }
      embs.push(emb)
      if (i < ENROLL_PROMPTS.length - 1) {
        enrollProgress.value = '准备下一段…'
        await sleep(600)
      }
    }
    const avg = SpeakerVerifier.averageEmbeddings(embs)
    enrollProgress.value = '上传声纹…'
    const status = await enrollVoiceprint({
      embedding: Array.from(avg),
      samples: embs.length,
    })
    voiceprintStatus.value = status
    enrollProgress.value = '录入成功'
    cacheVoiceprintEmbedding(Array.from(avg))
  } catch (e) {
    voiceprintError.value = e instanceof Error ? e.message : '录入失败'
  } finally {
    enrollingVoice.value = false
    enrollProgress.value = ''
  }
}

async function onDeleteVoiceprint() {
  if (!confirm('确定删除已录入的主人声纹吗？')) return
  voiceprintError.value = ''
  try {
    await deleteVoiceprint()
    voiceprintStatus.value = { enrolled: false }
    clearVoiceprintEmbeddingCache()
  } catch (e) {
    voiceprintError.value = e instanceof Error ? e.message : '删除失败'
  }
}

/** P2：主人面容录入（3 张快照，需开启「语音时看我」）。 */
async function startFaceprintEnroll() {
  if (enrollingFace.value) return
  if (!serverVisionEnabled.value) {
    faceprintError.value = '服务端未开启视觉能力'
    return
  }
  if (!isVisionCaptureEnabled()) {
    setVisionCaptureEnabled(true)
    visionEnabled.value = true
  }
  enrollingFace.value = true
  faceprintError.value = ''
  faceEnrollProgress.value = '初始化面容模型…'
  try {
    await enrollFaceVerifier.init()
    if (!enrollFaceVerifier.available) {
      faceprintError.value =
        '面容模型未就绪。请将 rec.onnx 放到 public/models/face/rec.onnx'
      return
    }
    const samples = getFaceprintConfig().enrollSamples || 3
    faceEnrollProgress.value = '打开 Mochi 的眼睛…'
    const visionResult = await startVisionSession()
    if (visionResult !== 'ok') {
      faceprintError.value = '无法看清画面，请允许摄像头并在设置中打开「语音时看我」'
      return
    }
    const jpegs: ArrayBuffer[] = []
    for (let i = 0; i < samples; i++) {
      faceEnrollProgress.value = `请正对 Mochi（${i + 1}/${samples}）…`
      await sleep(800)
      const buf = await visionSession.grabSnapshot()
      if (!buf) {
        faceprintError.value = `第 ${i + 1} 张抓拍失败，请调整光线后重试`
        return
      }
      jpegs.push(buf)
    }
    faceEnrollProgress.value = '分析面容…'
    const avg = await enrollFaceVerifier.enrollFromFrames(jpegs)
    if (!avg) {
      faceprintError.value = '面容分析失败，请正对镜头、光线充足后重试'
      return
    }
    faceEnrollProgress.value = '上传面容…'
    const status = await enrollFaceprint({
      embedding: Array.from(avg),
      samples: jpegs.length,
    })
    faceprintStatus.value = status
    faceEnrollProgress.value = '录入成功'
  } catch (e) {
    faceprintError.value = e instanceof Error ? e.message : '录入失败'
  } finally {
    await stopVisionSession().catch(() => {})
    enrollingFace.value = false
    faceEnrollProgress.value = ''
  }
}

async function onDeleteFaceprint() {
  if (!confirm('确定删除已录入的主人面容吗？')) return
  faceprintError.value = ''
  try {
    await deleteFaceprint()
    faceprintStatus.value = { enrolled: false }
  } catch (e) {
    faceprintError.value = e instanceof Error ? e.message : '删除失败'
  }
}

/** 重置 Tauri WebView2 麦克风站点权限（配合 Windows 隐私设置） */
async function onFixMicrophonePermission() {
  micFixBusy.value = true
  micFixMsg.value = ''
  try {
    const ok = await resetTauriMicrophonePermission()
    micFixMsg.value = ok
      ? '已重置应用内麦克风权限，请再试语音对话。'
      : micPermissionDeniedMessage()
  } finally {
    micFixBusy.value = false
  }
}

async function savePetName() {
  const name = petNameDraft.value.trim()
  if (!name) {
    nameError.value = '名字不能为空'
    return
  }
  savingName.value = true
  nameError.value = ''
  try {
    await updatePetName(name)
    pet.petName = name
  } catch (e) {
    nameError.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    savingName.value = false
  }
}

async function onDeleteMemory(id: number) {
  await growth.removeMemory(id)
}

async function onApproveEntry(id: number) {
  await growth.approvePendingEntry(id)
}

async function onRejectEntry(id: number) {
  await growth.rejectPendingEntry(id)
}

function logout() {
  if (!confirm('确定退出登录吗？')) return
  growth.closeSettings()
  auth.logout()
}

let unlistenTasksRefresh: (() => void) | null = null

onMounted(async () => {
  void loadServerVisionFlag()
  openMochiTab()
  unlistenTasksRefresh = await listenTasksRefresh(() => {
    if (tab.value === 'tasks') void loadTasks()
  })
})

onUnmounted(() => {
  unlistenTasksRefresh?.()
  unlistenTasksRefresh = null
})
</script>

<template>
  <div class="settings-root">
    <div class="settings-panel">
      <div class="settings-header">
        <span>设置</span>
        <button class="close-btn" type="button" aria-label="关闭" @click="close">✕</button>
      </div>

      <nav class="tabs">
        <button
          v-for="t in tabs"
          :key="t.id"
          type="button"
          :class="{ active: tab === t.id }"
          @click="selectTab(t.id)"
        >
          {{ t.label }}
        </button>
      </nav>

      <div class="panel-body">
        <div v-if="growth.loading && !['me', 'voice', 'tasks'].includes(tab)" class="loading">加载中…</div>

        <!-- Mochi：生命 + 关系 -->
        <template v-else-if="tab === 'mochi'">
          <SettingsCard title="概览">
            <div class="pet-head">
              <span class="gender-badge">{{ genderLabel(pet.gender) }}</span>
              <span v-if="pet.skuName" class="sku-tag">{{ pet.skuName }}</span>
            </div>
            <p class="life-line">
              {{ pet.lifecycle.life_stage_label }}
              · {{ pet.lifecycle.age_years }}岁{{ pet.lifecycle.age_days_in_year }}天
            </p>
            <p class="hint">还可陪伴 {{ pet.lifecycle.remaining_days }} 天</p>
          </SettingsCard>

          <SettingsCard v-if="growth.bond" title="你们的关系">
            <div class="stat">
              <span class="label">投缘度</span>
              <div class="bar-wrap">
                <div class="bar" :style="{ width: growth.bond.rapport_level + '%' }" />
              </div>
              <span class="value">{{ growth.bond.rapport_level }} · {{ rapportLabel(growth.bond.rapport_level) }}</span>
            </div>
            <div class="stat">
              <span class="label">信任度</span>
              <div class="bar-wrap trust">
                <div class="bar" :style="{ width: growth.bond.trust_level + '%' }" />
              </div>
              <span class="value">{{ growth.bond.trust_level }} · {{ trustLabel(growth.bond.trust_level) }}</span>
            </div>
            <p class="meta">
              已聊 {{ growth.bond.total_turns }} 轮
              <template v-if="growth.bond.streak_days > 1"> · 连续 {{ growth.bond.streak_days }} 天</template>
            </p>
            <template v-if="lastMoodLabel">
              <p class="hint">最近感受到你：{{ lastMoodLabel }}</p>
            </template>
            <template v-if="nicknames.user_calls_pet || nicknames.pet_calls_user">
              <p class="hint">
                你叫 TA「{{ nicknames.user_calls_pet || pet.petName }}」，TA 叫你「{{
                  nicknames.pet_calls_user || '主人'
                }}」
              </p>
            </template>
            <div v-if="topics.length" class="tags">
              <span v-for="t in topics" :key="t" class="tag">{{ t }}</span>
            </div>
            <p v-if="jokes.length" class="joke">你们的梗：{{ jokes[jokes.length - 1].content }}</p>
          </SettingsCard>

          <SettingsCard title="生命曲线">
            <div class="life-axis">
              <div
                v-for="(node, i) in LIFE_AXIS"
                :key="node.id"
                class="axis-node"
                :class="{ active: i === lifeAxisIndex, past: i < lifeAxisIndex }"
              >
                <span class="dot" />
                <span class="axis-label">{{ node.label }}</span>
              </div>
            </div>
            <p class="hint">{{ stageCompanionHint }} · {{ stageTeachingHint }}</p>
          </SettingsCard>

          <SettingsCard title="当前状态">
            <p class="life-line">
              心情 {{ pet.lifeState.mood }} · 亲密度 {{ pet.lifeState.love }} · 饥饿 {{ pet.lifeState.hungry }} ·
              精力 {{ pet.lifeState.energy }}
            </p>
          </SettingsCard>

          <SettingsCard v-if="pet.personality.traits || pet.personality.speech_style" title="人格">
            <p v-if="pet.personality.traits">性格：{{ pet.personality.traits }}</p>
            <p v-if="pet.personality.speech_style" class="hint">说话：{{ pet.personality.speech_style }}</p>
          </SettingsCard>

          <SettingsCard title="名字">
            <div class="name-row">
              <input v-model="petNameDraft" type="text" maxlength="32" placeholder="宠物名字" />
              <button type="button" class="primary-sm" :disabled="savingName" @click="savePetName">
                {{ savingName ? '…' : '保存' }}
              </button>
            </div>
            <p v-if="nameError" class="error">{{ nameError }}</p>
          </SettingsCard>
        </template>

        <!-- 记忆 -->
        <template v-else-if="tab === 'memory'">
          <SettingsCard v-if="growth.brief" title="长期画像">
            <div class="bar-wrap brief-bar">
              <div class="bar" :style="{ width: Math.min(100, (briefUsedChars / briefBudget) * 100) + '%' }" />
            </div>
            <p class="hint">约 {{ briefUsedChars }} / {{ briefBudget }} 字</p>
          </SettingsCard>

          <SettingsCard v-if="growth.writeApproval && growth.pendingBriefEntries.length" title="待确认画像">
            <p class="hint">确认后才会写入长期画像</p>
            <ul class="brief-list">
              <li v-for="e in growth.pendingBriefEntries" :key="e.id" class="pending-item">
                <div class="mem-meta">
                  <span class="cat">{{ CATEGORY_LABELS[e.category] || e.category }}</span>
                  <time v-if="e.created_at" class="time">{{ formatMemoryTime(e.created_at) }}</time>
                </div>
                <p class="pending-text">{{ e.content }}</p>
                <div class="pending-actions">
                  <button type="button" class="approve-btn" @click="onApproveEntry(e.id)">记住</button>
                  <button type="button" class="reject-btn" @click="onRejectEntry(e.id)">忽略</button>
                </div>
              </li>
            </ul>
          </SettingsCard>

          <SettingsCard v-if="growth.briefEntries.length" title="它记得关于你">
            <ul class="brief-list">
              <li v-for="e in growth.briefEntries" :key="e.id" class="chip-item">
                <div class="mem-meta">
                  <span class="cat">{{ CATEGORY_LABELS[e.category] || e.category }}</span>
                  <time v-if="e.created_at" class="time">{{ formatMemoryTime(e.created_at) }}</time>
                </div>
                {{ e.content }}
              </li>
            </ul>
          </SettingsCard>

          <SettingsCard v-if="growth.memories.length" title="记忆片段">
            <ul class="mem-list">
              <li v-for="m in growth.memories.slice(0, 12)" :key="m.id" class="mem-item">
                <div class="mem-bubble">
                  <div class="mem-meta">
                    <span class="cat">{{ m.type }}</span>
                    <time v-if="m.created_at" class="time">{{ formatMemoryTime(m.created_at) }}</time>
                  </div>
                  {{ m.content }}
                </div>
                <button type="button" class="del" title="删除" @click="onDeleteMemory(m.id)">×</button>
              </li>
            </ul>
          </SettingsCard>

          <p v-if="!growth.briefEntries.length && !growth.memories.length" class="empty">
            多聊几句，{{ pet.petName }} 会渐渐更懂你~
          </p>
        </template>

        <!-- 待办 -->
        <template v-else-if="tab === 'tasks'">
          <div v-if="tasksLoading" class="loading">加载中…</div>
          <template v-else>
            <p v-if="tasksError" class="error">{{ tasksError }}</p>

            <SettingsCard title="待提醒">
              <ul v-if="reminders.length" class="brief-list">
                <li v-for="r in reminders" :key="r.id" class="task-item">
                  <div>
                    <p class="task-title">{{ r.title }}</p>
                    <p class="hint">{{ formatTaskTime(r.fire_at) }}</p>
                  </div>
                  <button type="button" class="reject-btn" @click="onCancelReminder(r.id)">取消</button>
                </li>
              </ul>
              <p v-else class="hint">暂无提醒，聊天里跟 TA 说「明天9点提醒我…」</p>
            </SettingsCard>

            <SettingsCard title="待办">
              <ul v-if="todos.length" class="brief-list">
                <li v-for="t in todos" :key="t.id" class="task-item">
                  <div>
                    <p class="task-title">{{ t.title }}</p>
                    <p v-if="t.due_at" class="hint">{{ formatTaskTime(t.due_at) }}</p>
                  </div>
                  <button type="button" class="approve-btn" @click="onCompleteTodo(t.id)">完成</button>
                </li>
              </ul>
              <p v-else class="hint">暂无待办，可以说「帮我把买牛奶记下来」</p>
            </SettingsCard>

            <SettingsCard title="最近完成">
              <button type="button" class="link-btn" @click="showTaskHistory = !showTaskHistory">
                {{ showTaskHistory ? '收起' : '展开' }}
              </button>
              <template v-if="showTaskHistory">
                <ul v-if="doneReminders.length || doneTodos.length" class="brief-list history-list">
                  <li v-for="r in doneReminders" :key="'r' + r.id" class="chip-item muted">⏰ {{ r.title }}</li>
                  <li v-for="t in doneTodos" :key="'t' + t.id" class="chip-item muted">✓ {{ t.title }}</li>
                </ul>
                <p v-else class="hint">还没有完成记录~</p>
              </template>
            </SettingsCard>
          </template>
        </template>

        <!-- 声音 -->
        <template v-else-if="tab === 'voice'">
          <SettingsCard title="当前声音" hint="声音会随 TA 长大慢慢变化，不能手动换音色">
            <p class="life-line">{{ voiceProfile.title }}</p>
            <p class="hint">{{ voiceProfile.desc }}</p>
          </SettingsCard>

          <SettingsCard
            title="主人声纹"
            :badge="voiceprintStatus?.enrolled ? '已录入' : undefined"
            hint="对话时会持续验证你的声音；非主人会被拒答，你说话时会过滤他人声音"
          >
            <p v-if="voiceprintLoading" class="hint">加载声纹状态…</p>
            <p v-else-if="voiceprintStatus?.enrolled" class="hint">
              {{ voiceprintStatus.samples ?? 0 }} 段样本 · 维度 {{ voiceprintStatus.dim ?? 192 }}
            </p>
            <p v-else class="hint">尚未录入 — 无法开始语音对话</p>
            <p v-if="enrollProgress" class="hint">{{ enrollProgress }}</p>
            <button
              type="button"
              class="primary-sm full"
              :disabled="enrollingVoice"
              @click="startVoiceprintEnroll"
            >
              {{ enrollingVoice ? '录入中…' : voiceprintStatus?.enrolled ? '重新录入' : '录入主人声纹' }}
            </button>
            <button
              v-if="voiceprintStatus?.enrolled"
              type="button"
              class="reject-btn full enroll-del"
              :disabled="enrollingVoice"
              @click="onDeleteVoiceprint"
            >
              删除声纹
            </button>
            <p v-if="voiceprintError" class="error">{{ voiceprintError }}</p>
          </SettingsCard>

          <SettingsCard
            title="主人面容"
            :badge="faceprintStatus?.enrolled ? '已录入' : undefined"
            hint="与声纹配合认人；需开启「语音时看我」。看不清脸时不影响纯声纹对话"
          >
            <p v-if="faceprintLoading" class="hint">加载面容状态…</p>
            <p v-else-if="faceprintStatus?.enrolled" class="hint">
              {{ faceprintStatus.samples ?? 0 }} 张样本 · 维度 {{ faceprintStatus.dim ?? 512 }}
            </p>
            <p v-else class="hint">尚未录入 — 可选，用于降低声纹误拒</p>
            <p v-if="faceEnrollProgress" class="hint">{{ faceEnrollProgress }}</p>
            <button
              type="button"
              class="primary-sm full"
              :disabled="enrollingFace || !serverVisionEnabled"
              @click="startFaceprintEnroll"
            >
              {{ enrollingFace ? '录入中…' : faceprintStatus?.enrolled ? '重新录入' : '录入主人面容' }}
            </button>
            <button
              v-if="faceprintStatus?.enrolled"
              type="button"
              class="reject-btn full enroll-del"
              :disabled="enrollingFace"
              @click="onDeleteFaceprint"
            >
              删除面容
            </button>
            <p v-if="faceprintError" class="error">{{ faceprintError }}</p>
          </SettingsCard>

          <SettingsCard title="对话方式">
            <label class="toggle-row">
              <span>默认语音回复</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: voiceReplyDefault }"
                :disabled="savingProactive"
                @click="onVoiceReplyToggle"
              >
                {{ voiceReplyDefault ? '开' : '关' }}
              </button>
            </label>
            <template v-if="serverVisionEnabled">
              <label class="toggle-row">
                <span>语音时看我</span>
                <button type="button" class="toggle" :class="{ on: visionEnabled }" @click="onVisionToggle">
                  {{ visionEnabled ? '开' : '关' }}
                </button>
              </label>
              <p class="hint">通过「眼睛」理解表情、手里东西和周围；结束对话后关闭</p>
            </template>
          </SettingsCard>

          <SettingsCard v-if="isTauri()" title="麦克风" hint="若权限被拒绝，请检查 Windows 隐私 → 麦克风">
            <button
              type="button"
              class="primary-sm full"
              :disabled="micFixBusy"
              @click="onFixMicrophonePermission"
            >
              {{ micFixBusy ? '处理中…' : '修复麦克风权限' }}
            </button>
            <p v-if="micFixMsg" class="hint">{{ micFixMsg }}</p>
          </SettingsCard>

          <SettingsCard>
            <button type="button" class="settings-advanced-toggle" @click="voiceAdvancedOpen = !voiceAdvancedOpen">
              <span>高级</span>
              <span>{{ voiceAdvancedOpen ? '▲' : '▼' }}</span>
            </button>
            <div v-if="voiceAdvancedOpen" class="settings-advanced-body">
              <p class="hint">语音识别模式</p>
              <select v-model="sttMode" class="select-sm" @change="onSttModeChange">
                <option value="auto">自动</option>
                <option value="local">本地</option>
                <option value="cloud">云端</option>
              </select>
              <p v-if="sttMode === 'local' || sttMode === 'auto'" class="hint">
                本地模式优先连接本机 X-ASR（<code>ws://127.0.0.1:8766</code>）。
                请先运行 <code>tools/x-asr/setup-and-start.bat</code>；不可达时回退 Web Speech。
              </p>
              <p class="hint advanced-gap">在场声音感知 · 当前：{{ pet.ownerPresence }}</p>
              <p class="hint">
                模型路径：<code>public/models/speaker/campp.onnx</code>、
                <code>public/models/audio/yamnet.onnx</code>
              </p>
            </div>
          </SettingsCard>
        </template>

        <!-- 我的 -->
        <template v-else-if="tab === 'me'">
          <SettingsCard title="陪伴" hint="深夜我不吵你，重要提醒除外">
            <label class="toggle-row">
              <span>主动陪伴</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: proactiveEnabled }"
                :disabled="savingProactive"
                @click="onProactiveToggle"
              >
                {{ proactiveEnabled ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>看到我时聊聊</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: presenceChatEnabled }"
                :disabled="savingProactive || !proactiveEnabled"
                @click="onPresenceChatToggle"
              >
                {{ presenceChatEnabled ? '开' : '关' }}
              </button>
            </label>
            <p class="hint">摄像头确认你在面前时，Mochi 会主动找话题跟你聊（不是「想你了」那套）</p>
            <label class="toggle-row">
              <span>早安问候</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: morningGreeting }"
                :disabled="savingProactive"
                @click="onMorningToggle"
              >
                {{ morningGreeting ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>倾诉后关心</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: followUpEnabled }"
                :disabled="savingProactive"
                @click="onFollowUpToggle"
              >
                {{ followUpEnabled ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>提醒到点朗读</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: reminderVoice }"
                :disabled="savingProactive"
                @click="onReminderVoiceToggle"
              >
                {{ reminderVoice ? '开' : '关' }}
              </button>
            </label>
            <div class="quiet-row">
              <label>
                勿扰从
                <select v-model.number="quietStart" class="select-xs" @change="onQuietChange">
                  <option v-for="h in 24" :key="'s' + h" :value="h - 1">{{ h - 1 }}:00</option>
                </select>
              </label>
              <label>
                到
                <select v-model.number="quietEnd" class="select-xs" @change="onQuietChange">
                  <option v-for="h in 24" :key="'e' + h" :value="h - 1">{{ h - 1 }}:00</option>
                </select>
              </label>
            </div>
            <p v-if="prefsError" class="error">{{ prefsError }}</p>
          </SettingsCard>

          <SettingsCard title="生活照护" hint="根据键鼠使用情况提醒，不读取聊天内容">
            <label class="toggle-row">
              <span>总开关</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: wellnessEnabled }"
                :disabled="savingProactive"
                @click="onWellnessToggle"
              >
                {{ wellnessEnabled ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>喝水提醒</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: wellnessDrink }"
                :disabled="savingProactive || !wellnessEnabled"
                @click="onWellnessDrinkToggle"
              >
                {{ wellnessDrink ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>吃饭提醒</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: wellnessMeal }"
                :disabled="savingProactive || !wellnessEnabled"
                @click="onWellnessMealToggle"
              >
                {{ wellnessMeal ? '开' : '关' }}
              </button>
            </label>
            <label class="toggle-row">
              <span>休息 / 防过劳</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: wellnessRest }"
                :disabled="savingProactive || !wellnessEnabled"
                @click="onWellnessRestToggle"
              >
                {{ wellnessRest ? '开' : '关' }}
              </button>
            </label>
            <div class="quiet-row">
              <label>
                午餐
                <select v-model.number="lunchHour" class="select-xs" :disabled="!wellnessEnabled" @change="onMealTimeChange">
                  <option v-for="h in 24" :key="'lh' + h" :value="h - 1">{{ h - 1 }}:00</option>
                </select>
              </label>
              <label>
                晚餐
                <select v-model.number="dinnerHour" class="select-xs" :disabled="!wellnessEnabled" @change="onMealTimeChange">
                  <option v-for="h in 24" :key="'dh' + h" :value="h - 1">{{ h - 1 }}:00</option>
                </select>
              </label>
            </div>
            <label class="quiet-row">
              每日上限
              <select v-model.number="wellnessDailyMax" class="select-xs" :disabled="!wellnessEnabled" @change="onWellnessMaxChange">
                <option v-for="n in 4" :key="'wm' + n" :value="n">{{ n }} 条</option>
              </select>
            </label>
          </SettingsCard>

          <SettingsCard title="学习偏好">
            <div class="tags selectable">
              <button
                v-for="opt in topicOptions"
                :key="opt.id"
                type="button"
                class="tag tag-btn"
                :class="{ on: learningTopics.includes(opt.id) }"
                @click="toggleLearningTopic(opt.id)"
              >
                {{ opt.label }}
              </button>
            </div>
            <div class="radio-row">
              <span class="radio-label">英语</span>
              <label v-for="lv in englishLevels" :key="lv.id" class="radio-item">
                <input v-model="englishLevel" type="radio" :value="lv.id" />
                {{ lv.label }}
              </label>
            </div>
            <label class="toggle-row">
              <span>吐槽时不讲大道理</span>
              <button
                type="button"
                class="toggle"
                :class="{ on: noUnsolicitedAdvice }"
                @click="noUnsolicitedAdvice = !noUnsolicitedAdvice"
              >
                {{ noUnsolicitedAdvice ? '开' : '关' }}
              </button>
            </label>
            <button type="button" class="primary-sm full" :disabled="savingLearning" @click="saveLearningPrefs">
              {{ savingLearning ? '…' : '保存学习偏好' }}
            </button>
            <p v-if="learningError" class="error">{{ learningError }}</p>
          </SettingsCard>

          <SettingsCard title="关于我">
            <p class="hint">Mochi · v0.2</p>
          </SettingsCard>

          <SettingsCard title="账号" variant="danger">
            <p class="hint">当前已登录</p>
            <button type="button" class="danger" @click="logout">退出登录</button>
          </SettingsCard>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-root {
  position: relative;
  width: 100%;
  height: 100%;
  flex-shrink: 0;
  background: #fff;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.18);
}

.settings-panel {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.settings-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-weight: 600;
  font-size: 14px;
  flex-shrink: 0;
}

.close-btn {
  background: none;
  border: none;
  color: white;
  font-size: 15px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
  line-height: 1;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.2);
}

.tabs {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
  background: #fff;
}

.tabs button {
  flex: 1;
  padding: 8px 2px;
  border: none;
  border-radius: 0;
  background: transparent;
  font-size: 11px;
  color: #888;
  cursor: pointer;
}

.tabs button.active {
  background: transparent;
  color: var(--mochi-pink, #ff8fab);
  font-weight: 600;
  box-shadow: inset 0 -2px 0 var(--mochi-pink, #ff8fab);
}

.panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 12px 14px 14px;
  background: var(--mochi-bg, #fafafa);
}

.loading {
  text-align: center;
  color: #888;
  padding: 24px;
  font-size: 12px;
}

.stat {
  margin-bottom: 10px;
}

.stat .label {
  font-size: 12px;
  color: #666;
}

.bar-wrap {
  height: 6px;
  background: #f0f0f0;
  border-radius: 3px;
  margin: 4px 0;
  overflow: hidden;
}

.bar-wrap.trust .bar {
  background: #7eb6ff;
}

.bar {
  height: 100%;
  background: #ff8fab;
  border-radius: 3px;
}

.stat .value {
  font-size: 11px;
  color: #888;
}

.meta {
  font-size: 12px;
  color: #999;
  margin: 8px 0 0;
}

.block {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid #f0f0f0;
}

.block.flat {
  margin-top: 0;
  padding-top: 0;
  border-top: none;
}

.block h3 {
  margin: 0 0 8px;
  font-size: 12px;
  color: #888;
  font-weight: 600;
}

.block p,
.joke,
.hint,
.life-line {
  margin: 0;
  font-size: 13px;
  color: #333;
  line-height: 1.5;
}

.hint {
  color: #999;
  font-size: 12px;
  margin-top: 6px;
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tag {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 10px;
  background: #fff0f3;
  color: #c45;
}

.brief-list,
.mem-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chip-item {
  font-size: 12px;
  color: #444;
  line-height: 1.45;
  padding: 8px 10px;
  border-radius: 10px;
  background: #f3f3f3;
}

.mem-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}

.time {
  font-size: 10px;
  color: #aaa;
  white-space: nowrap;
  flex-shrink: 0;
}

.mem-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

.mem-bubble {
  flex: 1;
  font-size: 12px;
  color: #333;
  line-height: 1.45;
  padding: 8px 10px;
  border-radius: 12px;
  border-bottom-left-radius: 4px;
  background: #f3f3f3;
  word-break: break-word;
}

.cat {
  display: inline-block;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background: rgba(255, 143, 171, 0.15);
  color: #c45;
  margin-right: 4px;
}

.del {
  flex-shrink: 0;
  border: none;
  background: none;
  color: #ccc;
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  padding: 4px;
  border-radius: 6px;
}

.del:hover {
  color: #e74c3c;
  background: #fff5f5;
}

.empty {
  text-align: center;
  color: #aaa;
  font-size: 12px;
  margin-top: 32px;
  line-height: 1.6;
}

.name-row {
  display: flex;
  gap: 8px;
}

.name-row input {
  flex: 1;
  padding: 8px 10px;
  border: 1px solid #eee;
  border-radius: 10px;
  font-size: 13px;
  outline: none;
}

.name-row input:focus {
  border-color: #ffb3c6;
}

.primary-sm {
  padding: 8px 12px;
  border: none;
  border-radius: 10px;
  background: #ff8fab;
  color: white;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.primary-sm:disabled {
  opacity: 0.5;
  cursor: default;
}

.error {
  color: #e74c3c;
  font-size: 12px;
  margin-top: 6px;
}

.danger {
  margin-top: 8px;
  width: 100%;
  padding: 10px;
  border: none;
  border-radius: 12px;
  background: #fff5f5;
  color: #c0392b;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}

.danger:hover {
  background: #ffe8e8;
}

.pending-block {
  margin-bottom: 12px;
}

.pending-item {
  padding: 10px;
  border-radius: 10px;
  background: #fff8e6;
  border: 1px solid #ffe8a3;
}

.pending-text {
  margin: 4px 0 8px;
  font-size: 12px;
  color: #444;
  line-height: 1.45;
}

.pending-actions {
  display: flex;
  gap: 8px;
}

.approve-btn,
.reject-btn {
  flex: 1;
  padding: 6px 8px;
  border-radius: 8px;
  border: none;
  font-size: 12px;
  cursor: pointer;
}

.approve-btn {
  background: #ff8fab;
  color: white;
  font-weight: 600;
}

.reject-btn {
  background: #f0f0f0;
  color: #666;
}

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: #333;
}

.toggle {
  min-width: 44px;
  padding: 6px 10px;
  border: none;
  border-radius: 999px;
  background: #ddd;
  color: #666;
  font-size: 12px;
  cursor: pointer;
}

.toggle.on {
  background: #ff8fab;
  color: white;
  font-weight: 600;
}

.toggle:disabled {
  opacity: 0.6;
  cursor: default;
}

.task-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px;
  border-radius: 10px;
  background: #f8f8f8;
}

.task-title {
  margin: 0;
  font-size: 13px;
  color: #333;
  line-height: 1.4;
}

.brief-bar {
  margin-top: 6px;
}

.pet-head {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.gender-badge,
.sku-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 999px;
  background: #fff0f3;
  color: #c45;
}

.life-axis {
  display: flex;
  justify-content: space-between;
  gap: 2px;
  margin: 8px 0 10px;
}

.axis-node {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.axis-node .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ddd;
}

.axis-node.past .dot,
.axis-node.active .dot {
  background: #ff8fab;
}

.axis-node.active .axis-label {
  color: #e05;
  font-weight: 600;
}

.axis-label {
  font-size: 10px;
  color: #999;
  white-space: nowrap;
}

.tags.selectable {
  margin-bottom: 8px;
}

.tag-btn {
  border: 1px solid #ffd0dc;
  cursor: pointer;
}

.tag-btn.on {
  background: #ff8fab;
  color: white;
}

.radio-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin: 8px 0;
  font-size: 12px;
}

.radio-label {
  color: #888;
  margin-right: 4px;
}

.radio-item {
  display: flex;
  align-items: center;
  gap: 3px;
}

.primary-sm.full {
  width: 100%;
  margin-top: 8px;
}

.select-sm,
.select-xs {
  border: 1px solid #eee;
  border-radius: 8px;
  padding: 6px 8px;
  font-size: 12px;
  background: #fff;
}

.quiet-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
  font-size: 12px;
  color: #666;
}

.link-btn {
  border: none;
  background: none;
  color: #e05;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}

.history-list {
  margin-top: 8px;
}

.chip-item.muted {
  color: #999;
  font-size: 11px;
}

.enroll-del {
  margin-top: 8px;
  width: 100%;
}

.advanced-gap {
  margin-top: 10px;
}

.toggle-row + .toggle-row {
  margin-top: 8px;
}
</style>
