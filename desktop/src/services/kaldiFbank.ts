/**
 * Kaldi-compatible 80-dim log-fbank for 3D-Speaker / CAM++ ONNX models.
 * Matches sherpa-onnx scripts/3dspeaker/test-onnx.py defaults.
 */
const SAMPLE_RATE = 16000
const FRAME_LENGTH = 400 // 25 ms @ 16 kHz
const FRAME_SHIFT = 160 // 10 ms @ 16 kHz
const N_FFT = 512
const NUM_MELS = 80
const PREEMPH = 0.97
const LOW_FREQ = 20
const HIGH_FREQ = 8000 // 0 in Kaldi means Nyquist; 16 kHz → 8000 Hz

/** Compute (T, 80) fbank features from mono PCM in [-1, 1]. */
export function computeKaldiFbank(pcm: Float32Array): Float32Array {
  if (pcm.length < FRAME_LENGTH) return new Float32Array(0)

  const preemph = preemphasis(pcm)
  const numFrames = 1 + Math.floor((preemph.length - FRAME_LENGTH) / FRAME_SHIFT)
  if (numFrames <= 0) return new Float32Array(0)

  const melFilters = buildMelFilters(N_FFT, SAMPLE_RATE, NUM_MELS, LOW_FREQ, HIGH_FREQ)
  const window = poveyWindow(FRAME_LENGTH)
  const out = new Float32Array(numFrames * NUM_MELS)

  for (let f = 0; f < numFrames; f++) {
    const start = f * FRAME_SHIFT
    const frame = new Float32Array(N_FFT)
    for (let i = 0; i < FRAME_LENGTH; i++) {
      frame[i] = preemph[start + i] * window[i]
    }
    removeDcOffset(frame, FRAME_LENGTH)

    const power = powerSpectrum(frame, N_FFT)
    let melOffset = f * NUM_MELS
    for (let m = 0; m < NUM_MELS; m++) {
      let sum = 0
      const filter = melFilters[m]
      for (let k = 0; k < filter.length; k++) {
        sum += power[k] * filter[k]
      }
      out[melOffset + m] = Math.log(Math.max(sum, 1e-10))
    }
  }

  return out
}

/** Per-utterance global mean normalization (CAM++ / 3D-Speaker). */
export function applyGlobalMeanNorm(flat: Float32Array, numFrames: number, dim = NUM_MELS): Float32Array {
  if (numFrames <= 0) return flat
  const mean = new Float32Array(dim)
  for (let f = 0; f < numFrames; f++) {
    const base = f * dim
    for (let d = 0; d < dim; d++) mean[d] += flat[base + d]
  }
  for (let d = 0; d < dim; d++) mean[d] /= numFrames

  const out = new Float32Array(flat.length)
  for (let f = 0; f < numFrames; f++) {
    const base = f * dim
    for (let d = 0; d < dim; d++) out[base + d] = flat[base + d] - mean[d]
  }
  return out
}

/** Flat (T*80) → [1, T, 80] row-major for ONNX input `x`. */
export function packFbankBatch(flat: Float32Array, numFrames: number, dim = NUM_MELS): Float32Array {
  return flat.slice(0, numFrames * dim)
}

export function fbankFrameCount(pcmLength: number): number {
  if (pcmLength < FRAME_LENGTH) return 0
  return 1 + Math.floor((pcmLength - FRAME_LENGTH) / FRAME_SHIFT)
}

function preemphasis(samples: Float32Array): Float32Array {
  const out = new Float32Array(samples.length)
  out[0] = samples[0]
  for (let i = 1; i < samples.length; i++) {
    out[i] = samples[i] - PREEMPH * samples[i - 1]
  }
  return out
}

function poveyWindow(length: number): Float32Array {
  const w = new Float32Array(length)
  const denom = length - 1
  for (let i = 0; i < length; i++) {
    const h = 0.5 - 0.5 * Math.cos((2 * Math.PI * i) / denom)
    w[i] = Math.pow(h, 0.85)
  }
  return w
}

function removeDcOffset(frame: Float32Array, length: number) {
  let mean = 0
  for (let i = 0; i < length; i++) mean += frame[i]
  mean /= length
  for (let i = 0; i < length; i++) frame[i] -= mean
}

function powerSpectrum(frame: Float32Array, nFft: number): Float32Array {
  const re = new Float32Array(nFft)
  const im = new Float32Array(nFft)
  re.set(frame.subarray(0, Math.min(frame.length, nFft)))
  fft(re, im, nFft)

  const bins = nFft / 2 + 1
  const power = new Float32Array(bins)
  for (let k = 0; k < bins; k++) {
    power[k] = re[k] * re[k] + im[k] * im[k]
  }
  return power
}

function buildMelFilters(
  nFft: number,
  sampleRate: number,
  nMels: number,
  lowFreq: number,
  highFreq: number,
): Float32Array[] {
  const fftBins = nFft / 2 + 1
  const melLow = hzToMel(lowFreq)
  const melHigh = hzToMel(highFreq)
  const melPoints = new Float32Array(nMels + 2)
  for (let i = 0; i < nMels + 2; i++) {
    melPoints[i] = melLow + ((melHigh - melLow) * i) / (nMels + 1)
  }
  const hzPoints = new Float32Array(nMels + 2)
  for (let i = 0; i < nMels + 2; i++) hzPoints[i] = melToHz(melPoints[i])

  const bin = new Int32Array(nMels + 2)
  for (let i = 0; i < nMels + 2; i++) {
    bin[i] = Math.floor(((nFft + 1) * hzPoints[i]) / sampleRate)
    if (bin[i] >= fftBins) bin[i] = fftBins - 1
  }

  const filters: Float32Array[] = []
  for (let m = 0; m < nMels; m++) {
    const f = new Float32Array(fftBins)
    for (let k = bin[m]; k < bin[m + 1]; k++) {
      if (bin[m + 1] - bin[m] > 0) {
        f[k] = (k - bin[m]) / (bin[m + 1] - bin[m])
      }
    }
    for (let k = bin[m + 1]; k < bin[m + 2]; k++) {
      if (bin[m + 2] - bin[m + 1] > 0) {
        f[k] = (bin[m + 2] - k) / (bin[m + 2] - bin[m + 1])
      }
    }
    filters.push(f)
  }
  return filters
}

function hzToMel(hz: number): number {
  return 1127 * Math.log(1 + hz / 700)
}

function melToHz(mel: number): number {
  return 700 * (Math.exp(mel / 1127) - 1)
}

function fft(re: Float32Array, im: Float32Array, n: number) {
  if (n <= 1) return
  let j = 0
  for (let i = 0; i < n; i++) {
    if (i < j) {
      ;[re[i], re[j]] = [re[j], re[i]]
      ;[im[i], im[j]] = [im[j], im[i]]
    }
    let m = n >> 1
    while (m >= 1 && j >= m) {
      j -= m
      m >>= 1
    }
    j += m
  }

  for (let len = 2; len <= n; len <<= 1) {
    const ang = (-2 * Math.PI) / len
    const wlenRe = Math.cos(ang)
    const wlenIm = Math.sin(ang)
    for (let i = 0; i < n; i += len) {
      let wRe = 1
      let wIm = 0
      for (let k = 0; k < len / 2; k++) {
        const uRe = re[i + k]
        const uIm = im[i + k]
        const vRe = re[i + k + len / 2] * wRe - im[i + k + len / 2] * wIm
        const vIm = re[i + k + len / 2] * wIm + im[i + k + len / 2] * wRe
        re[i + k] = uRe + vRe
        im[i + k] = uIm + vIm
        re[i + k + len / 2] = uRe - vRe
        im[i + k + len / 2] = uIm - vIm
        const nextWRe = wRe * wlenRe - wIm * wlenIm
        wIm = wRe * wlenIm + wIm * wlenRe
        wRe = nextWRe
      }
    }
  }
}
