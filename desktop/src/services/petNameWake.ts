/**
 * 主人呼喊桌宠名字时的唤醒/门控匹配（含 ASR 常见谐音）。
 */
import { usePetStore } from '@/stores/petStore'
import { useGrowthStore } from '@/stores/growthStore'
import { parseNicknames } from '@/types/growth'

/** 名字谐音/误识别变体（ASR 常把「卡卡」听成近音字） */
const NAME_ALIASES: Record<string, string[]> = {
  卡卡: ['咔咔', '佳佳', '喀喀', '卡卡的', '咔咔的'],
  mochi: ['mochi', '莫奇', '摸奇'],
}

function normalize(s: string): string {
  return s.trim().toLowerCase()
}

/** 收集主人对桌宠的所有称呼（设置名 + bond 昵称，去重） */
export function getPetCallNames(): string[] {
  const pet = usePetStore()
  const growth = useGrowthStore()
  const nn = parseNicknames(growth.bond?.nicknames)
  const raw = [pet.petName, nn.user_calls_pet].map((s) => s?.trim()).filter(Boolean) as string[]
  const seen = new Set<string>()
  const out: string[] = []
  for (const n of raw) {
    const key = normalize(n)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(n)
  }
  return out
}

/** 文本是否包含对桌宠的呼喊（名字或已知谐音） */
export function textContainsPetName(text: string): boolean {
  const t = text.trim()
  if (!t) return false
  const lower = t.toLowerCase()
  for (const name of getPetCallNames()) {
    if (!name) continue
    if (t.includes(name) || lower.includes(name.toLowerCase())) return true
    const aliases = NAME_ALIASES[name] ?? NAME_ALIASES[name.toLowerCase()] ?? []
    for (const alias of aliases) {
      if (t.includes(alias) || lower.includes(alias.toLowerCase())) return true
    }
  }
  return false
}
