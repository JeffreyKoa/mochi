import { resolveXTtsFetchBase, synthesizeXTts } from '@/services/xTtsClient'
import { stripMoodTags } from '@/utils/stripMoodTags'

const STRONG_PUNCT = /[。！？!?；;]/
const WEAK_PUNCT = /[，,、]/

/** 按句切分（中英标点），供整段合成。 */
export function splitTtsSentences(text: string): string[] {
  const clean = stripMoodTags(text).trim()
  if (!clean) return []

  const parts: string[] = []
  let buf = ''
  for (const ch of clean) {
    buf += ch
    if (STRONG_PUNCT.test(ch)) {
      const seg = buf.trim()
      if (seg) parts.push(seg)
      buf = ''
    }
  }
  const tail = buf.trim()
  if (tail) parts.push(tail)
  return parts.length > 0 ? parts : [clean]
}

/** 从 buffer[offset..] 提取已完成的句子（流式 LLM → TTS）。 */
export function extractFlushableSentences(
  text: string,
  offset: number,
  isFirst: boolean,
): { sentences: string[]; newOffset: number } {
  const clean = stripMoodTags(text)
  if (offset >= clean.length) return { sentences: [], newOffset: offset }

  const sentences: string[] = []
  let scanFrom = offset
  let first = isFirst

  while (scanFrom < clean.length) {
    let cutAt = -1
    for (let i = scanFrom; i < clean.length; i++) {
      const ch = clean[i]!
      if (STRONG_PUNCT.test(ch)) {
        cutAt = i + 1
        break
      }
      if (first && WEAK_PUNCT.test(ch)) {
        const seg = clean.slice(scanFrom, i + 1).trim()
        if (seg.length >= 6) {
          cutAt = i + 1
          break
        }
      }
    }
    if (cutAt < 0) break
    const seg = clean.slice(scanFrom, cutAt).trim()
    if (seg) sentences.push(seg)
    scanFrom = cutAt
    first = false
  }

  return { sentences, newOffset: scanFrom }
}

/** 流水线合成：合成 N+1 与播放 N 重叠。 */
export async function synthesizeLocalSpeechSegments(
  baseUrl: string,
  text: string,
  onSegment: (wav: ArrayBuffer, index: number) => void,
): Promise<boolean> {
  const segments = splitTtsSentences(text)
  if (segments.length === 0) return false

  const base = resolveXTtsFetchBase(baseUrl)
  let ok = false
  let nextPromise: Promise<ArrayBuffer | null> | null = synthesizeXTts(base, segments[0]!)

  for (let i = 0; i < segments.length; i++) {
    const wav = await nextPromise!
    nextPromise = i + 1 < segments.length ? synthesizeXTts(base, segments[i + 1]!) : null
    if (!wav || wav.byteLength === 0) continue
    ok = true
    onSegment(wav, i)
  }
  return ok
}

/** 整段合成（短句快捷路径）。 */
export async function synthesizeLocalSpeech(
  baseUrl: string,
  text: string,
): Promise<ArrayBuffer | null> {
  const clean = stripMoodTags(text).trim()
  if (!clean) return null
  return synthesizeXTts(resolveXTtsFetchBase(baseUrl), clean)
}

/** 流式本地 TTS：随 llm_token 增量切句合成。 */
export class LocalTtsStreamer {
  private buffer = ''
  private offset = 0
  private segmentIndex = 0
  private flushedCount = 0
  private chain: Promise<void> = Promise.resolve()
  private cancelled = false
  gotAudio = false

  constructor(
    private baseUrl: string,
    private onSegment: (wav: ArrayBuffer, index: number) => void,
  ) {}

  /** 追加 LLM token。 */
  append(token: string) {
    if (this.cancelled || !token) return
    this.buffer += token
    const { sentences, newOffset } = extractFlushableSentences(
      this.buffer,
      this.offset,
      this.flushedCount === 0,
    )
    if (sentences.length === 0) return
    this.offset = newOffset
    for (const seg of sentences) {
      this.flushedCount++
      this.scheduleSynth(seg)
    }
  }

  /** 合成剩余文本并等待队列排空。 */
  async finish(): Promise<boolean> {
    if (this.cancelled) return this.gotAudio
    const tail = stripMoodTags(this.buffer.slice(this.offset)).trim()
    if (tail) {
      this.offset = this.buffer.length
      this.scheduleSynth(tail)
    }
    await this.chain
    return this.gotAudio
  }

  cancel() {
    this.cancelled = true
  }

  private scheduleSynth(sentence: string) {
    const idx = this.segmentIndex++
    const base = resolveXTtsFetchBase(this.baseUrl)
    this.chain = this.chain.then(async () => {
      if (this.cancelled) return
      const wav = await synthesizeXTts(base, sentence)
      if (this.cancelled || !wav || wav.byteLength === 0) return
      this.gotAudio = true
      this.onSegment(wav, idx)
    })
  }
}
