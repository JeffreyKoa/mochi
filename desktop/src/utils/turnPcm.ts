import { float32ToPcm16LE } from '@/services/pcmCapture'

/** 将 float32 PCM 快照编码为 base64 int16 LE（供 text_input.turn_pcm 声学识别）。 */
export function encodeTurnPcmBase64(samples: Float32Array): string {
  if (samples.length === 0) return ''
  const buf = float32ToPcm16LE(samples)
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]!)
  }
  return btoa(binary)
}
