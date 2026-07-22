import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export interface LifeState {
  mood: number
  love: number
  hungry: number
  energy: number
}

export type Animation = 'idle' | 'happy' | 'sad' | 'sleep' | 'eat' | 'walk'

export const usePetStore = defineStore('pet', () => {
  const petName = ref('Mochi')
  const lifeState = ref<LifeState>({ mood: 70, love: 60, hungry: 30, energy: 80 })
  const currentAnimation = ref<Animation>('idle')
  const facing = ref<'left' | 'right'>('right')
  const isRoaming = ref(false)
  const isChatOpen = ref(false)
  const bubbleText = ref('')
  const showBubble = ref(false)

  const moodEmoji = computed(() => {
    const mood = lifeState.value.mood
    if (mood >= 80) return '😄'
    if (mood >= 50) return '😊'
    if (mood >= 30) return '😐'
    return '😢'
  })

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

  function setFacing(dir: 'left' | 'right') {
    facing.value = dir
  }

  function setRoaming(v: boolean) {
    isRoaming.value = v
  }

  return {
    petName,
    lifeState,
    currentAnimation,
    facing,
    isRoaming,
    isChatOpen,
    bubbleText,
    showBubble,
    moodEmoji,
    updateLifeState,
    setAnimation,
    setFacing,
    setRoaming,
    showSpeechBubble,
    syncAnimationFromState,
  }
})
