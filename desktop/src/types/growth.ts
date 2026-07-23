export interface BondNicknames {
  user_calls_pet?: string
  pet_calls_user?: string
}

export interface BondInsideJoke {
  content: string
  created_at?: string
}

export interface BondProfile {
  pet_id: number
  rapport_level: number
  trust_level: number
  shared_topics: string | string[]
  nicknames: string | BondNicknames
  inside_jokes: string | BondInsideJoke[]
  last_mood_tag?: string
  total_turns: number
  streak_days: number
  updated_at?: string
}

export interface UserBriefEntry {
  id: number
  pet_id: number
  category: string
  content: string
  importance: number
  source: string
  status?: string
  created_at?: string
}

export interface MemoryItem {
  id: number
  type: string
  content: string
  importance: number
  created_at?: string
}

export interface UserBrief {
  pet_id: number
  compiled_text: string
  compiled_at?: string
  char_budget: number
}

export interface OnboardingInput {
  user_calls_pet?: string
  pet_calls_user?: string
  traits?: string
  speech_style?: string
  first_topic?: string
  first_joke?: string
}

export function parseNicknames(raw: string | BondNicknames | undefined): BondNicknames {
  if (!raw) return {}
  if (typeof raw === 'object') return raw
  try {
    return JSON.parse(raw) as BondNicknames
  } catch {
    return {}
  }
}

export function parseInsideJokes(raw: string | BondInsideJoke[] | undefined): BondInsideJoke[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function parseSharedTopics(raw: string | string[] | undefined): string[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function needsOnboarding(bond: BondProfile, petId: number): boolean {
  if (localStorage.getItem(`mochi_onboarding_done_${petId}`)) return false
  const nn = parseNicknames(bond.nicknames)
  const topics = parseSharedTopics(bond.shared_topics)
  const hasNickname = !!(nn.user_calls_pet || nn.pet_calls_user)
  return !hasNickname && topics.length === 0
}

export function markOnboardingDone(petId: number) {
  localStorage.setItem(`mochi_onboarding_done_${petId}`, '1')
}

export const CATEGORY_LABELS: Record<string, string> = {
  preference: '偏好',
  habit: '习惯',
  taboo: '雷区',
  style: '风格',
  person: '人物',
}
