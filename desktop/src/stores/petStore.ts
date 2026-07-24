import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  DEFAULT_SKIN_COLORS,
  parsePetSkin,
  skinColorsToPixi,
  hexToPixi,
  type PetSkin,
  type PetSkinColors,
} from '@/types/petSkin'

export interface LifeState {
  mood: number
  love: number
  hungry: number
  energy: number
}

export interface PetLifecycle {
  species: string
  breed: string
  life_stage: string
  life_stage_label: string
  age_days: number
  age_years: number
  age_days_in_year: number
  remaining_days: number
  max_days: number
  is_alive: boolean
}

export type Animation = 'idle' | 'happy' | 'sad' | 'sleep' | 'eat' | 'walk'

export const usePetStore = defineStore('pet', () => {
  const petName = ref('Mochi')
  const lifeState = ref<LifeState>({ mood: 70, love: 60, hungry: 30, energy: 80 })
  const lifecycle = ref<PetLifecycle>({
    species: 'cat',
    breed: '',
    life_stage: 'newborn',
    life_stage_label: '刚出生',
    age_days: 0,
    age_years: 0,
    age_days_in_year: 0,
    remaining_days: 6570,
    max_days: 6570,
    is_alive: true,
  })
  const skuId = ref('')
  const skuName = ref('')
  const skin = ref<PetSkin | null>(null)
  const currentAnimation = ref<Animation>('idle')
  const facing = ref<'left' | 'right'>('right')
  const isRoaming = ref(false)
  const isChatOpen = ref(false)
  const bubbleText = ref('')
  const showBubble = ref(false)
  const bootFailed = ref(false)
  let retryBootFn: (() => void) | null = null

  function registerBootRetry(fn: () => void) {
    retryBootFn = fn
  }

  function retryBoot() {
    if (retryBootFn) retryBootFn()
  }

  function setBootFailed(failed: boolean) {
    bootFailed.value = failed
  }

  const moodEmoji = computed(() => {
    const mood = lifeState.value.mood
    if (mood >= 80) return '😄'
    if (mood >= 50) return '😊'
    if (mood >= 30) return '😐'
    return '😢'
  })

  const skinColors = computed((): PetSkinColors => skin.value?.colors ?? DEFAULT_SKIN_COLORS)

  const animationColors = computed(() => skinColorsToPixi(skinColors.value))

  const legColor = computed(() => hexToPixi(skinColors.value.leg))
  const footColor = computed(() => hexToPixi(skinColors.value.foot))
  const earInnerColor = computed(() => hexToPixi(skinColors.value.ear_inner))

  function applySkinFromSKU(sku?: { sku_id?: string; name?: string; skin?: unknown }) {
    if (!sku) return
    if (sku.sku_id) skuId.value = sku.sku_id
    if (sku.name) skuName.value = sku.name
    const parsed = parsePetSkin(sku.skin)
    if (parsed) skin.value = parsed
  }

  function updateLifecycle(partial: Partial<PetLifecycle>) {
    Object.assign(lifecycle.value, partial)
  }

  function updateLifeState(partial: Partial<LifeState>) {
    Object.assign(lifeState.value, partial)
  }

  function setAnimation(anim: Animation) {
    currentAnimation.value = anim
  }

  function showSpeechBubble(text: string, duration = 4000) {
    bubbleText.value = text
    showBubble.value = true
    setTimeout(() => {
      showBubble.value = false
    }, duration)
  }

  function showPersistentBubble(text: string) {
    bubbleText.value = text
    showBubble.value = true
  }

  function hideSpeechBubble() {
    showBubble.value = false
  }

  function syncAnimationFromState() {
    if (isRoaming.value) return
    const { mood, energy } = lifeState.value
    if (energy < 10) {
      currentAnimation.value = 'sleep'
    } else if (mood < 30) {
      currentAnimation.value = 'sad'
    } else if (currentAnimation.value === 'sleep' || currentAnimation.value === 'sad') {
      currentAnimation.value = 'idle'
    }
  }

  function getWakeGreeting(): string {
    const anim = currentAnimation.value
    if (anim === 'sleep') return '嗯… 我醒了，主人有啥吩咐~'
    if (anim === 'sad') return '我在呢… 主人说~'
    return '我在这里，主人有啥吩咐~'
  }

  function setFacing(dir: 'left' | 'right') {
    facing.value = dir
  }

  function setRoaming(v: boolean) {
    isRoaming.value = v
  }

  return {
    petName,
    lifeState,
    lifecycle,
    skuId,
    skuName,
    skin,
    skinColors,
    animationColors,
    legColor,
    footColor,
    earInnerColor,
    currentAnimation,
    facing,
    isRoaming,
    isChatOpen,
    bubbleText,
    showBubble,
    bootFailed,
    moodEmoji,
    updateLifeState,
    updateLifecycle,
    applySkinFromSKU,
    setAnimation,
    setFacing,
    setRoaming,
    showSpeechBubble,
    showPersistentBubble,
    hideSpeechBubble,
    registerBootRetry,
    retryBoot,
    setBootFailed,
    syncAnimationFromState,
    getWakeGreeting,
  }
})
