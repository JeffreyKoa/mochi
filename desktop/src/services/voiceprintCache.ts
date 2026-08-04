/**
 * 主人声纹 embedding 本地缓存（设置页录入 + 语音对话共用）。
 * 历史上设置页用 mochi_owner_embedding，对话侧用 mochi_voiceprint_embedding，需兼容两者。
 */
const VOICEPRINT_EMBED_KEYS = ['mochi_voiceprint_embedding', 'mochi_owner_embedding'] as const

/** 写入本地缓存（双 key，兼容旧版设置页）。 */
export function cacheVoiceprintEmbedding(embedding: number[]) {
  const json = JSON.stringify(embedding)
  for (const key of VOICEPRINT_EMBED_KEYS) {
    try {
      localStorage.setItem(key, json)
    } catch {
      // ignore quota
    }
  }
}

/** 读取本地缓存的 embedding；任一键存在即可。 */
export function readCachedVoiceprintEmbedding(): Float32Array | null {
  for (const key of VOICEPRINT_EMBED_KEYS) {
    try {
      const raw = localStorage.getItem(key)
      if (!raw) continue
      const arr = JSON.parse(raw) as number[]
      if (Array.isArray(arr) && arr.length > 0) return new Float32Array(arr)
    } catch {
      // try next key
    }
  }
  return null
}

/** 清除本地声纹缓存。 */
export function clearVoiceprintEmbeddingCache() {
  for (const key of VOICEPRINT_EMBED_KEYS) {
    try {
      localStorage.removeItem(key)
    } catch {
      // ignore
    }
  }
}
