/**
 * 拉取 ONNX 模型并校验，避免 Vite SPA 回退 HTML（200 + text/html）被当作模型解析。
 */

const MIN_ONNX_BYTES = 1024

/** 常见 HTML/JSON 误响应前缀 */
function looksLikeNonOnnx(bytes: Uint8Array): boolean {
  if (bytes.length < 4) return true
  const head = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3])
  if (head.startsWith('<!DO') || head.startsWith('<htm') || head.startsWith('<?xm')) return true
  if (bytes[0] === 0x7b /* { */) return true // JSON error page
  return false
}

function isLikelyOnnxContentType(ct: string | null): boolean {
  if (!ct) return true // 部分静态服务器不送 Content-Type
  const lower = ct.toLowerCase()
  if (lower.includes('text/html')) return false
  if (lower.includes('application/json')) return false
  return true
}

export class OnnxFetchError extends Error {
  constructor(
    message: string,
    readonly url: string,
  ) {
    super(message)
    this.name = 'OnnxFetchError'
  }
}

/** 下载并校验 ONNX；失败时抛出 OnnxFetchError。 */
export async function fetchOnnxArrayBuffer(url: string): Promise<ArrayBuffer> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new OnnxFetchError(`HTTP ${res.status}`, url)
  }
  const ct = res.headers.get('content-type')
  if (!isLikelyOnnxContentType(ct)) {
    throw new OnnxFetchError(`unexpected content-type: ${ct ?? '(none)'}`, url)
  }
  const buf = await res.arrayBuffer()
  const view = new Uint8Array(buf)
  if (view.length < MIN_ONNX_BYTES) {
    throw new OnnxFetchError(`file too small (${view.length} bytes)`, url)
  }
  if (looksLikeNonOnnx(view)) {
    const preview = new TextDecoder().decode(view.subarray(0, 64)).replace(/\s+/g, ' ').trim()
    throw new OnnxFetchError(`not an ONNX file (head=${preview.slice(0, 40)}...)`, url)
  }
  return buf
}
