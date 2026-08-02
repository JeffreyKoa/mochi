/**
 * 客户端视觉启发式（仅用于决定何时补拍帧，不参与服务端 V3c 路由）。
 */

/** 举物 / 指物 / 认物类口语片段（partial ASR 命中即可能补拍 object_refresh 帧）。 */
const OBJECT_QUERY_HINTS = [
  '这是什么',
  '这是啥',
  '什么玩意',
  '什么东西',
  '啥东西',
  '帮我看看',
  '看看这个',
  '认认这个',
  '认一下',
  '我手里',
  '手里拿',
  '拿着',
  '举着',
  '拿的什么',
  '拿的啥',
  'what is this',
  'what\'s this',
]

/** 单字触发需与上下文组合，避免误触（如「这个好」）。 */
const OBJECT_QUERY_CHARS = ['这', '看', '啥', '物']

/**
 * 判断 ASR 文本是否像「举物 / 指物 / 认物」问句。
 * 供 P2 在 speech 中途或 submit 前补拍 object_refresh 帧。
 */
export function looksLikeObjectQuery(text: string): boolean {
  const t = text.trim().toLowerCase()
  if (!t) return false
  if (OBJECT_QUERY_HINTS.some((h) => t.includes(h.toLowerCase()))) return true
  // 「这/看/啥/物」至少出现 2 个，降低单字误触
  const charHits = OBJECT_QUERY_CHARS.filter((c) => t.includes(c)).length
  return charHits >= 2
}
