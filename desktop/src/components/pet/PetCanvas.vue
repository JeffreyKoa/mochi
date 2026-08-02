<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import * as PIXI from 'pixi.js'
import { usePetStore } from '@/stores/petStore'
import type { Animation } from '@/stores/petStore'

const pet = usePetStore()
const canvasRef = ref<HTMLCanvasElement>()

const CANVAS_W = 280
const CANVAS_H = 280
const BODY_R = 48
const BODY_CY = 18

let app: PIXI.Application | null = null
let petGraphic: PIXI.Graphics | null = null
let animTimer: ReturnType<typeof setInterval> | null = null
let bounceOffset = 0
let legSwing = 0
let earWiggle = 0
let tailSwing = 0
let tailWave = 0
let blinkPhase = 0
let eyeLook = 0
/** 尾扇三连跳：-1 未播放，0..N 播放中 */
let burstFrame = -1
const BURST_LAST_FRAME = 25

type BurstFace = 'default' | 'hop' | 'smirk'
let burstFace: BurstFace = 'default'

const COLORS = computed(() => pet.animationColors)
const LEG_COLOR = computed(() => pet.legColor)
const FOOT_COLOR = computed(() => pet.footColor)
const EAR_INNER = computed(() => pet.earInnerColor)

let lastLegColor = 0xff7aa2
let lastFootColor = 0xd63384
let lastEarInner = 0xff9eb5

/** 竖耳兔耳：默认挺立，仅 sad/sleep 时略垂。 */
function drawEars(cy: number, color: number, droop = 0) {
  if (!petGraphic) return

  const g = petGraphic
  const dL = droop
  const dR = droop

  // 左耳（竖立 teardrop）
  g.moveTo(-22, 2 + cy)
  g.bezierCurveTo(-38 + dL * 0.3, -20 + cy, -42 + dL * 0.5, -52 + cy, -30 + dL * 0.4, -74 + cy)
  g.bezierCurveTo(-24 + dL * 0.25, -78 + cy, -14 + dL * 0.15, -72 + cy, -8, -48 + cy)
  g.bezierCurveTo(-4, -28 + cy, -10, -6 + cy, -22, 2 + cy)
  g.closePath()
  g.fill(color)

  // 右耳
  g.moveTo(22, 2 + cy)
  g.bezierCurveTo(38 + dR * 0.3, -20 + cy, 42 + dR * 0.5, -52 + cy, 30 + dR * 0.4, -74 + cy)
  g.bezierCurveTo(24 + dR * 0.25, -78 + cy, 14 + dR * 0.15, -72 + cy, 8, -48 + cy)
  g.bezierCurveTo(4, -28 + cy, 10, -6 + cy, 22, 2 + cy)
  g.closePath()
  g.fill(color)
}

function drawEarInner(cy: number, droop = 0) {
  if (!petGraphic) return

  const g = petGraphic
  const dL = droop
  const dR = droop

  g.moveTo(-24, -6 + cy)
  g.bezierCurveTo(-32 + dL * 0.25, -26 + cy, -34 + dL * 0.35, -48 + cy, -28 + dL * 0.3, -62 + cy)
  g.bezierCurveTo(-22 + dL * 0.2, -58 + cy, -16, -38 + cy, -18, -16 + cy)
  g.closePath()
  g.fill({ color: lastEarInner, alpha: 0.9 })

  g.moveTo(24, -6 + cy)
  g.bezierCurveTo(32 + dR * 0.25, -26 + cy, 34 + dR * 0.35, -48 + cy, 28 + dR * 0.3, -62 + cy)
  g.bezierCurveTo(22 + dR * 0.2, -58 + cy, 16, -38 + cy, 18, -16 + cy)
  g.closePath()
  g.fill({ color: lastEarInner, alpha: 0.9 })
}

/** Build tail puff positions — thin at root, slightly thicker toward the tip. */
function buildTailSegments(cy: number, swing: number, wave: number) {
  const s = swing
  const w = wave
  const curl = 1 + Math.max(s, -14) * 0.012
  const radii = [7, 7.5, 8.5, 9.5, 10, 10.5, 11, 11.5, 12.5]

  return [
    { x: 34 + s * 0.15, y: cy + 16, r: radii[0] },
    { x: 37 + s * 0.35, y: cy + 2, r: radii[1] },
    { x: 40 + s * 0.65 + w * 0.25, y: cy - 14 * curl, r: radii[2] },
    { x: 42 + s * 1.0 + w * 0.45, y: cy - 28 * curl, r: radii[3] },
    { x: 43 + s * 1.3 + w * 0.6, y: cy - 40 * curl, r: radii[4] },
    { x: 42 + s * 1.6 + w * 0.75, y: cy - 52 * curl, r: radii[5] },
    { x: 40 + s * 1.85 + w * 0.88, y: cy - 62 * curl, r: radii[6] },
    { x: 38 + s * 2.05 + w * 1.0, y: cy - 70 * curl, r: radii[7] },
    { x: 36 + s * 2.25 + w * 1.1, y: cy - 76 * curl + w * 0.15, r: radii[8] },
  ]
}

function drawTailSegments(
  segments: ReturnType<typeof buildTailSegments>,
  color: number,
  from: number,
  to: number,
  withHighlights: boolean,
) {
  if (!petGraphic) return
  const g = petGraphic

  for (let i = from; i <= to; i++) {
    const seg = segments[i]
    g.circle(seg.x, seg.y, seg.r)
    g.fill(color)
  }

  if (!withHighlights) return

  for (let i = Math.max(from, 1); i <= to; i++) {
    if (i >= segments.length - 1) continue
    const seg = segments[i]
    g.circle(seg.x - 4, seg.y + 3, seg.r * 0.52)
    g.fill({ color: lastEarInner, alpha: 0.38 })
  }

  if (to >= segments.length - 1) {
    const tip = segments[segments.length - 1]
    g.circle(tip.x - 3, tip.y + 2, tip.r * 0.45)
    g.fill({ color: lastEarInner, alpha: 0.5 })
  }
}

function drawTailBase(cy: number, color: number, swing: number, wave: number) {
  const segments = buildTailSegments(cy, swing, wave)
  drawTailSegments(segments, color, 0, 2, false)
}

function drawTailUpper(cy: number, color: number, swing: number, wave: number) {
  const segments = buildTailSegments(cy, swing, wave)
  drawTailSegments(segments, color, 3, segments.length - 1, true)
}

function drawLegs(legTop: number) {
  if (!petGraphic) return
  const swing = legSwing
  const legW = 14
  const legH = 30

  // Left leg + foot
  petGraphic.roundRect(-20 + swing, legTop, legW, legH, 5)
  petGraphic.fill(lastLegColor)
  petGraphic.ellipse(-13 + swing, legTop + legH + 1, 11, 8)
  petGraphic.fill(lastFootColor)

  // Right leg + foot
  petGraphic.roundRect(6 - swing, legTop, legW, legH, 5)
  petGraphic.fill(lastLegColor)
  petGraphic.ellipse(13 - swing, legTop + legH + 1, 11, 8)
  petGraphic.fill(lastFootColor)
}

function drawBodyHighlight(cy: number, r: number) {
  if (!petGraphic) return
  petGraphic.circle(-14, cy - r * 0.35, r * 0.55)
  petGraphic.fill({ color: 0xffffff, alpha: 0.14 })
  petGraphic.circle(-8, cy - r * 0.55, r * 0.22)
  petGraphic.fill({ color: 0xffffff, alpha: 0.22 })
}

/** 眼神：随动画切换眼型，idle 带高光与微视线。 */
function drawEyes(cy: number, anim: Animation, eyeOpen: boolean) {
  if (!petGraphic) return
  const g = petGraphic
  const lx = -18 + eyeLook * 0.6
  const rx = 18 + eyeLook * 0.6
  const ey = -6 + cy

  if (!eyeOpen || anim === 'sleep') {
    g.moveTo(lx - 9, ey)
    g.quadraticCurveTo(lx, ey - 3, lx + 9, ey)
    g.moveTo(rx - 9, ey)
    g.quadraticCurveTo(rx, ey - 3, rx + 9, ey)
    g.stroke({ width: 2.8, color: 0x333333, cap: 'round' })
    return
  }

  if (anim === 'happy' || anim === 'eat') {
    g.moveTo(lx - 10, ey - 2)
    g.quadraticCurveTo(lx, ey - 12, lx + 10, ey - 2)
    g.moveTo(rx - 10, ey - 2)
    g.quadraticCurveTo(rx, ey - 12, rx + 10, ey - 2)
    g.stroke({ width: 3, color: 0x333333, cap: 'round' })
    g.circle(lx + 4, ey - 6, 2)
    g.circle(rx + 4, ey - 6, 2)
    g.fill({ color: 0xffffff, alpha: 0.85 })
    return
  }

  if (anim === 'sad') {
    g.moveTo(lx - 9, ey - 4)
    g.quadraticCurveTo(lx, ey + 4, lx + 9, ey - 4)
    g.moveTo(rx - 9, ey - 4)
    g.quadraticCurveTo(rx, ey + 4, rx + 9, ey - 4)
    g.stroke({ width: 2.5, color: 0x333333, cap: 'round' })
    g.circle(lx + 1, ey + 2, 3.5)
    g.circle(rx + 1, ey + 2, 3.5)
    g.fill(0x333333)
    return
  }

  // idle / walk：圆眼 + 大高光 + 上眼线
  g.ellipse(lx, ey, 8, 9)
  g.ellipse(rx, ey, 8, 9)
  g.fill(0xffffff)
  g.circle(lx + 1, ey + 1, 5.5)
  g.circle(rx + 1, ey + 1, 5.5)
  g.fill(0x333333)
  g.circle(lx + 3, ey - 2, 2.2)
  g.circle(rx + 3, ey - 2, 2.2)
  g.fill(0xffffff)
  g.circle(lx - 1, ey + 3, 1)
  g.circle(rx - 1, ey + 3, 1)
  g.fill({ color: 0xffffff, alpha: 0.55 })
  g.moveTo(lx - 9, ey - 10)
  g.quadraticCurveTo(lx, ey - 13, lx + 9, ey - 10)
  g.moveTo(rx - 9, ey - 10)
  g.quadraticCurveTo(rx, ey - 13, rx + 9, ey - 10)
  g.stroke({ width: 1.5, color: 0x333333, alpha: 0.35, cap: 'round' })
}

/** 嘴型：idle 翘嘴，happy 大笑；burst 定格用 smirk。 */
function drawMouth(cy: number, anim: Animation, face: BurstFace = 'default') {
  if (!petGraphic) return
  const g = petGraphic
  const my = 10 + cy

  if (pet.isSpeaking) {
    const mouthH = Math.max(4, Math.min(16, pet.lipSyncVolume * 18))
    g.ellipse(0, my - 2, 9, mouthH)
    g.fill(0x333333)
    g.ellipse(0, my + mouthH * 0.3, 7, mouthH * 0.35)
    g.fill({ color: 0xff6b8a, alpha: 0.45 })
    return
  }

  if (face === 'smirk') {
    g.moveTo(-12, my - 2)
    g.quadraticCurveTo(3, my + 7, 14, my - 4)
    g.stroke({ width: 2.8, color: 0x333333, cap: 'round' })
    return
  }

  if (anim === 'happy') {
    g.arc(0, my - 4, 14, 0.15, Math.PI - 0.15)
    g.stroke({ width: 2.8, color: 0x333333, cap: 'round' })
    g.circle(0, my + 2, 3)
    g.fill({ color: 0xff6b8a, alpha: 0.7 })
    return
  }

  if (anim === 'eat') {
    g.ellipse(0, my, 10, 7)
    g.fill(0x333333)
    g.ellipse(0, my + 3, 8, 4)
    g.fill({ color: 0xff6b8a, alpha: 0.5 })
    return
  }

  if (anim === 'sad') {
    g.moveTo(-10, my - 2)
    g.quadraticCurveTo(0, my + 8, 10, my - 2)
    g.stroke({ width: 2.5, color: 0x333333, cap: 'round' })
    return
  }

  if (anim === 'sleep') {
    g.moveTo(-6, my)
    g.quadraticCurveTo(0, my + 3, 6, my)
    g.stroke({ width: 2, color: 0x333333, alpha: 0.5, cap: 'round' })
    return
  }

  // idle / walk：不对称翘嘴
  g.moveTo(-11, my - 1)
  g.quadraticCurveTo(2, my + 6, 13, my - 3)
  g.stroke({ width: 2.6, color: 0x333333, cap: 'round' })
}

function applyDanceTransform(anim: Animation) {
  if (!petGraphic) return

  if (anim === 'happy') {
    petGraphic.rotation = 0
    petGraphic.skew.set(0, 0)
    petGraphic.scale.set(pet.facing === 'left' ? -1 : 1, 1)
    return
  }

  petGraphic.rotation = 0
  petGraphic.skew.set(0, 0)
  petGraphic.scale.set(pet.facing === 'left' ? -1 : 1, 1)
}

/** 尾扇三连跳 + 翘嘴定格：~1.3s @50ms */
function sampleHappyBurst(frame: number): {
  bounce: number
  legSwing: number
  earWiggle: number
  tailSwing: number
  tailWave: number
  scale: number
  face: BurstFace
} | null {
  if (frame < 0 || frame > BURST_LAST_FRAME) return null
  if (frame <= 1) {
    return { bounce: 0, legSwing: 0, earWiggle: -4, tailSwing: 2, tailWave: 0, scale: 1.02, face: 'hop' }
  }
  if (frame <= 4) {
    return { bounce: -7, legSwing: 16, earWiggle: -3, tailSwing: 10, tailWave: 5, scale: 1.05, face: 'hop' }
  }
  if (frame <= 7) {
    return { bounce: -11, legSwing: -16, earWiggle: -2, tailSwing: 14, tailWave: 7, scale: 1.06, face: 'hop' }
  }
  if (frame <= 10) {
    return { bounce: -15, legSwing: 18, earWiggle: -2, tailSwing: 20, tailWave: 9, scale: 1.08, face: 'hop' }
  }
  if (frame <= 18) {
    const t = frame - 11
    const fan = Math.sin(t * 1.15) * 14
    return {
      bounce: -6 + Math.abs(Math.sin(t * 0.85)) * -4,
      legSwing: fan * 0.45,
      earWiggle: -1,
      tailSwing: 28 + fan,
      tailWave: 16 + Math.sin(t * 0.9) * 5,
      scale: 1.05,
      face: 'hop',
    }
  }
  return {
    bounce: -8,
    legSwing: 0,
    earWiggle: -2,
    tailSwing: 30,
    tailWave: 14,
    scale: 1.05,
    face: 'smirk',
  }
}

function applyHappyBurstFrame(frame: number): boolean {
  const sample = sampleHappyBurst(frame)
  if (!sample) {
    burstFace = 'default'
    return false
  }
  bounceOffset = sample.bounce
  legSwing = sample.legSwing
  earWiggle = sample.earWiggle
  tailSwing = sample.tailSwing
  tailWave = sample.tailWave
  burstFace = sample.face
  eyeLook = 0
  return true
}

function drawPet(color: number, scale = 1, eyeOpen = true, face: BurstFace = burstFace) {
  if (!petGraphic) return
  petGraphic.clear()

  const anim = pet.currentAnimation
  const cy = BODY_CY + bounceOffset
  const r = BODY_R * scale
  const legTop = cy + r - 6
  const earDroop = anim === 'sad' ? 6 + earWiggle : anim === 'sleep' ? 4 : earWiggle

  drawTailBase(cy, color, tailSwing, tailWave)
  drawLegs(legTop)
  drawEars(cy, color, earDroop)

  petGraphic.circle(0, cy, r)
  petGraphic.fill(color)
  drawBodyHighlight(cy, r)

  drawEarInner(cy, earDroop)
  drawTailUpper(cy, color, tailSwing, tailWave)

  drawEyes(cy, anim, eyeOpen)
  drawMouth(cy, anim, face)

  const blushAlpha =
    face === 'smirk' ? 0.58 : anim === 'happy' ? 0.45 : anim === 'walk' || anim === 'idle' ? 0.28 : 0
  if (blushAlpha > 0) {
    petGraphic.circle(-32, 6 + cy, 10)
    petGraphic.circle(32, 6 + cy, 10)
    petGraphic.fill({ color: 0xff6b8a, alpha: blushAlpha })
  }
  if (anim === 'sad') {
    petGraphic.ellipse(-28, 14 + cy, 3, 5)
    petGraphic.ellipse(28, 14 + cy, 3, 5)
    petGraphic.fill({ color: 0x6eb5ff, alpha: 0.55 })
  }
}

function startAnimLoop(anim: Animation) {
  if (animTimer) clearInterval(animTimer)
  let frame = 0

  animTimer = setInterval(() => {
    frame++
    const color = COLORS.value[anim]
    lastLegColor = LEG_COLOR.value
    lastFootColor = FOOT_COLOR.value
    lastEarInner = EAR_INNER.value

    switch (anim) {
      case 'idle':
        bounceOffset = Math.sin(frame * 0.15) * 2
        legSwing = Math.sin(frame * 0.14) * 4
        earWiggle = Math.sin(frame * 0.18) * 1.5
        tailSwing = Math.sin(frame * 0.1) * 6
        tailWave = Math.sin(frame * 0.16 + 1.2) * 3.5
        eyeLook = Math.sin(frame * 0.08) * 2
        blinkPhase = frame % 120
        break
      case 'happy':
        if (burstFrame >= 0) {
          const ok = applyHappyBurstFrame(burstFrame)
          if (ok) {
            drawPet(color, sampleHappyBurst(burstFrame)!.scale, true, burstFace)
            applyDanceTransform('happy')
            burstFrame++
            if (burstFrame > BURST_LAST_FRAME) {
              burstFrame = -1
              burstFace = 'default'
            }
            return
          }
          burstFrame = -1
          burstFace = 'default'
        }
        bounceOffset = Math.abs(Math.sin(frame * 0.55)) * -9
        legSwing = Math.sin(frame * 0.9) * 12
        earWiggle = Math.sin(frame * 0.85) * 3
        tailSwing = Math.sin(frame * 0.65) * 16
        tailWave = Math.sin(frame * 0.85 + 0.8) * 8
        eyeLook = 0
        burstFace = 'default'
        break
      case 'sad':
        bounceOffset = 3
        legSwing = 0
        earWiggle = 4 + Math.sin(frame * 0.05) * 1
        tailSwing = Math.sin(frame * 0.06) * 3 - 14
        tailWave = Math.sin(frame * 0.08) * 2
        eyeLook = 0
        break
      case 'sleep':
        bounceOffset = Math.sin(frame * 0.08) * 1.5
        legSwing = 0
        earWiggle = 5
        tailSwing = Math.sin(frame * 0.05) * 2 - 10
        tailWave = 0
        eyeLook = 0
        break
      case 'eat':
        bounceOffset = Math.sin(frame * 0.5) * 2
        legSwing = Math.sin(frame * 0.5) * 5
        earWiggle = Math.sin(frame * 0.55) * 2
        tailSwing = Math.sin(frame * 0.4) * 10
        tailWave = Math.sin(frame * 0.52) * 4.5
        eyeLook = 0
        break
      case 'walk':
        legSwing = Math.sin(frame * 0.55) * 18
        bounceOffset = Math.abs(Math.sin(frame * 0.55)) * 5
        earWiggle = Math.sin(frame * 0.6) * 2.5
        tailSwing = Math.sin(frame * 0.55) * 16
        tailWave = Math.sin(frame * 0.72 + 0.5) * 6.5
        eyeLook = Math.sin(frame * 0.2) * 1.5
        break
    }

    const eyeOpen =
      anim === 'sleep'
        ? true
        : anim === 'eat'
          ? frame % 10 < 7
          : anim === 'idle'
            ? blinkPhase < 115 || blinkPhase > 118
            : true
    const scale =
      anim === 'happy' ? 1 + Math.sin(frame * 0.3) * 0.04
      : anim === 'walk' ? 1 + Math.sin(frame * 0.55) * 0.02
      : anim === 'sad' ? 0.96
      : 1

    drawPet(color, scale, eyeOpen)
    applyDanceTransform(anim)
  }, 50)
}

watch(() => pet.happyBurstSeq, () => {
  burstFrame = 0
  burstFace = 'default'
  if (pet.currentAnimation !== 'happy') {
    pet.setAnimation('happy')
  }
})

watch(() => pet.animationColors, () => {
  drawPet(COLORS.value[pet.currentAnimation], 1, pet.currentAnimation !== 'sleep')
  applyDanceTransform(pet.currentAnimation)
}, { deep: true })

onMounted(async () => {
  if (!canvasRef.value) return

  app = new PIXI.Application()
  await app.init({
    canvas: canvasRef.value,
    width: CANVAS_W,
    height: CANVAS_H,
    backgroundAlpha: 0,
    antialias: true,
    resolution: window.devicePixelRatio || 1,
    autoDensity: true,
  })

  petGraphic = new PIXI.Graphics()
  petGraphic.pivot.set(0, BODY_CY)
  petGraphic.x = CANVAS_W / 2
  petGraphic.y = CANVAS_H / 2 + BODY_CY
  petGraphic.scale.x = pet.facing === 'left' ? -1 : 1
  app.stage.addChild(petGraphic)

  startAnimLoop(pet.currentAnimation)
})

watch(() => pet.currentAnimation, (anim) => {
  startAnimLoop(anim)
})

watch(() => pet.facing, () => {
  applyDanceTransform(pet.currentAnimation)
})

onUnmounted(() => {
  if (animTimer) clearInterval(animTimer)
  app?.destroy(true)
})
</script>

<template>
  <canvas ref="canvasRef" class="pet-canvas" />
</template>

<style scoped>
.pet-canvas {
  width: 280px;
  height: 280px;
  display: block;
  pointer-events: none;
}
</style>
