<script setup lang="ts">
import { ref } from 'vue'
import { useGrowthStore } from '@/stores/growthStore'
import { usePetStore } from '@/stores/petStore'

const emit = defineEmits<{ done: [] }>()
const growth = useGrowthStore()
const pet = usePetStore()

const step = ref(1)
const userCallsPet = ref('')
const petCallsUser = ref('主人')
const firstTopic = ref('')
const firstJoke = ref('')
const loading = ref(false)
const error = ref('')

async function next() {
  error.value = ''
  if (step.value === 1) {
    if (!userCallsPet.value.trim()) {
      error.value = '给它起个称呼吧~'
      return
    }
    step.value = 2
    return
  }
  if (step.value === 2) {
    step.value = 3
    return
  }
  await finish()
}

function back() {
  error.value = ''
  if (step.value > 1) step.value--
}

async function finish() {
  loading.value = true
  error.value = ''
  try {
    await growth.submitOnboarding({
      user_calls_pet: userCallsPet.value.trim(),
      pet_calls_user: petCallsUser.value.trim() || '主人',
      first_topic: firstTopic.value.trim() || undefined,
      first_joke: firstJoke.value.trim() || undefined,
    })
    if (userCallsPet.value.trim()) {
      pet.petName = userCallsPet.value.trim()
    }
    emit('done')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败，再试一次~'
  } finally {
    loading.value = false
  }
}

async function skip() {
  growth.skipOnboarding()
  emit('done')
}
</script>

<template>
  <aside class="side-panel onboarding-panel">
    <div class="panel-inner">
      <div class="steps">
        <span v-for="n in 3" :key="n" :class="{ active: step >= n }" />
      </div>

      <template v-if="step === 1">
        <h2>第一次见面 🍡</h2>
        <p class="hint">跟屁虫想认识你呢~ 怎么互相称呼？</p>
        <label>
          <span>你叫它</span>
          <input v-model="userCallsPet" type="text" maxlength="8" placeholder="团子" />
        </label>
        <label>
          <span>它叫你</span>
          <input v-model="petCallsUser" type="text" maxlength="8" placeholder="主人" />
        </label>
      </template>

      <template v-else-if="step === 2">
        <h2>常聊什么？</h2>
        <p class="hint">选一个你们可能会聊的话题（可跳过）</p>
        <input v-model="firstTopic" type="text" maxlength="32" placeholder="游戏、猫、工作吐槽…" />
      </template>

      <template v-else>
        <h2>第一个梗</h2>
        <p class="hint">可选：留一句只属于你们的小记忆</p>
        <input v-model="firstJoke" type="text" maxlength="64" placeholder="比如：你又熬夜！" />
      </template>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="actions">
        <button v-if="step > 1" type="button" class="ghost" @click="back">上一步</button>
        <button v-if="step < 3" type="button" class="primary" @click="next">下一步</button>
        <button v-else type="button" class="primary" :disabled="loading" @click="finish">
          {{ loading ? '保存中…' : '开始相处' }}
        </button>
      </div>
      <button type="button" class="skip" @click="skip">先跳过</button>
    </div>
  </aside>
</template>

<style scoped>
.side-panel {
  width: 320px;
  height: 100%;
  flex-shrink: 0;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: -6px 0 24px rgba(0, 0, 0, 0.14);
  border-left: 1px solid rgba(0, 0, 0, 0.06);
  overflow: hidden;
}

.panel-inner {
  height: 100%;
  overflow-y: auto;
  padding: 20px 18px 24px;
  box-sizing: border-box;
}

.steps {
  display: flex;
  gap: 6px;
  justify-content: center;
  margin-bottom: 16px;
}

.steps span {
  width: 28px;
  height: 4px;
  border-radius: 2px;
  background: #eee;
}

.steps span.active {
  background: #ff8fab;
}

h2 {
  font-size: 17px;
  color: #333;
  margin: 0 0 6px;
  text-align: center;
}

.hint {
  font-size: 13px;
  color: #888;
  text-align: center;
  margin: 0 0 16px;
  line-height: 1.4;
}

label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 12px;
  font-size: 12px;
  color: #666;
}

input {
  padding: 10px 12px;
  border: 1px solid #e5e5e5;
  border-radius: 10px;
  font-size: 14px;
  outline: none;
  width: 100%;
  box-sizing: border-box;
}

input:focus {
  border-color: #ff8fab;
}

.actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}

button.primary {
  flex: 1;
  padding: 10px;
  border: none;
  border-radius: 10px;
  background: linear-gradient(135deg, #ff8fab, #ffb3c6);
  color: white;
  font-weight: 600;
  cursor: pointer;
}

button.ghost {
  padding: 10px 14px;
  border: 1px solid #ddd;
  border-radius: 10px;
  background: white;
  cursor: pointer;
}

button.skip {
  display: block;
  width: 100%;
  margin-top: 10px;
  border: none;
  background: none;
  color: #aaa;
  font-size: 12px;
  cursor: pointer;
}

.error {
  color: #e74c3c;
  font-size: 12px;
  text-align: center;
  margin: 8px 0 0;
}
</style>
