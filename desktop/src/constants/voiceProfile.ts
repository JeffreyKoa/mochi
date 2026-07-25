import { axisIndexForStage, type LifeAxisId } from './lifecycle'

export interface VoiceProfileLabel {
  title: string
  desc: string
}

function stageBucket(stage: string): LifeAxisId {
  const idx = axisIndexForStage(stage)
  const ids: LifeAxisId[] = ['newborn', 'juvenile', 'youth', 'prime', 'elder']
  return ids[idx] ?? 'newborn'
}

export function resolveVoiceLabel(gender: string, lifeStage: string): VoiceProfileLabel {
  const g = gender === 'male' ? 'male' : 'female'
  const bucket = stageBucket(lifeStage)

  if (g === 'male') {
    switch (bucket) {
      case 'newborn':
      case 'juvenile':
        return { title: '少年音 · 偏亮', desc: '阳光、有活力，语速略快。' }
      case 'youth':
        return { title: '青年音 · 清朗', desc: '自然有精神，情感表达充分。' }
      case 'prime':
        return { title: '壮年音 · 沉稳', desc: '磁性温暖，语速稳。' }
      case 'elder':
        return { title: '老年音 · 低缓', desc: '温暖慢语，像懂你的老朋友。' }
    }
  }

  switch (bucket) {
    case 'newborn':
    case 'juvenile':
      return { title: '少女音 · 偏软', desc: '幼态亲和，句子短而软。' }
    case 'youth':
      return { title: '青年音 · 清亮', desc: '亲和清晰，适合日常对话。' }
    case 'prime':
      return { title: '壮年音 · 知性', desc: '稳而清晰，办事时也很靠谱。' }
    case 'elder':
      return { title: '老年音 · 温柔', desc: '慢而包容，有阅历感。' }
  }
}
