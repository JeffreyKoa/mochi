/** 合法 mood 名（与 server/internal/text/tone.go 一致） */
const MOOD_NAMES = 'gentle|excited|sad|calm|worried|playful|serious'

/** 标准格式：[mood:gentle] */
const MOOD_TAG_RE = new RegExp(`\\[mood:\\s*(?:${MOOD_NAMES})\\s*\\]`, 'gi')
/** 流式/漏标残缺：[mood:pla、m] */
const MOOD_TAG_PARTIAL_RE = /\[mood:[^\]]*\]?/gi
/** LLM 常误写为 [gentle]（缺 mood: 前缀） */
const BARE_MOOD_TAG_RE = new RegExp(`\\[(?:${MOOD_NAMES})\\]`, 'gi')
/** 残缺尾部：playful]、gentle] */
const BARE_MOOD_ORPHAN_RE = new RegExp(`(?:${MOOD_NAMES})\\]`, 'gi')
/** 句首孤立 ]（上一 token 被截断） */
const STRAY_LEAD_BRACKET_RE = /^\s*\]\s*/

export function stripMoodTags(text: string): string {
  if (!text) return ''
  return text
    .replace(MOOD_TAG_RE, '')
    .replace(MOOD_TAG_PARTIAL_RE, '')
    .replace(BARE_MOOD_TAG_RE, '')
    .replace(BARE_MOOD_ORPHAN_RE, '')
    .replace(STRAY_LEAD_BRACKET_RE, '')
    .replace(/\s{2,}/g, ' ')
    .trim()
}
