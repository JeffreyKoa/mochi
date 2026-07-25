/** User-facing life stage axis (maps internal stages to display nodes). */

export type LifeAxisId = 'newborn' | 'juvenile' | 'youth' | 'prime' | 'elder'

export interface LifeAxisNode {
  id: LifeAxisId
  label: string
  stages: string[]
}

export const LIFE_AXIS: LifeAxisNode[] = [
  { id: 'newborn', label: '出生', stages: ['newborn'] },
  { id: 'juvenile', label: '少年', stages: ['juvenile', 'child'] },
  { id: 'youth', label: '青年', stages: ['youth'] },
  { id: 'prime', label: '壮年', stages: ['prime'] },
  { id: 'elder', label: '老年', stages: ['elder', 'twilight'] },
]

export function axisIndexForStage(stage: string): number {
  const s = stage || 'newborn'
  const idx = LIFE_AXIS.findIndex((n) => n.stages.includes(s))
  return idx >= 0 ? idx : 0
}

export function companionHint(stage: string): string {
  switch (stage) {
    case 'newborn':
      return '黏人、话少，以陪伴为主。'
    case 'juvenile':
    case 'child':
      return '好奇爱闹，像小伙伴一起探索。'
    case 'youth':
      return '活跃爱聊，最能帮你想事情。'
    case 'prime':
      return '稳重靠谱，办事最熟练。'
    case 'elder':
    case 'twilight':
      return '温和记得久，少打扰、多倾听。'
    default:
      return '自然陪伴你。'
  }
}

export function teachingHint(stage: string): string {
  switch (stage) {
    case 'newborn':
      return '几乎不主动说教，先陪着。'
    case 'juvenile':
    case 'child':
      return '短句举例，英语/学习游戏化。'
    case 'youth':
      return '场景对话、处世「你可以试试…」'
    case 'prime':
      return '系统讲解，利弊分析清楚。'
    case 'elder':
    case 'twilight':
      return '阅历与分寸，走心少布置任务。'
    default:
      return '你主动要学时，我再教。'
  }
}

export function genderLabel(gender: string): string {
  return gender === 'male' ? '男 Mochi' : '女 Mochi'
}

export const MOOD_LABELS: Record<string, string> = {
  happy: '开心',
  sad: '有点难过',
  anxious: '焦虑',
  stressed: '有压力',
  tired: '疲惫',
  angry: '生气',
  neutral: '平常',
}
