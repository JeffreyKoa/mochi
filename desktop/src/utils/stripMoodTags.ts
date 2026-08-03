/** 剥离 LLM 输出的 [mood:xxx] 标记，避免 UI 出现 m] 等残留 */
const MOOD_TAG_RE = /\[mood:\s*(?:gentle|excited|sad|calm|worried|playful|serious)\s*\]/gi
const MOOD_TAG_PARTIAL_RE = /\[mood:[^\]]*\]?/gi

export function stripMoodTags(text: string): string {
  if (!text) return ''
  return text
    .replace(MOOD_TAG_RE, '')
    .replace(MOOD_TAG_PARTIAL_RE, '')
    .replace(/\s{2,}/g, ' ')
    .trim()
}
