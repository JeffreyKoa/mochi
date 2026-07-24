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
const BODY_CY = -18
/** Base scale during happy spin — further reduced dynamically when tail nears edges. */
const DANCE_FIT_SCALE = 0.7
const DANCE_EDGE_MARGIN = 18
/** Max distance from body pivot to tail tip (incl. puff radius). */
const TAIL_EXTENT = 138
const LEG_EXTENT = 84

let app: PIXI.Application | null = null
let petGraphic: PIXI.Graphics | null = null
let animTimer: ReturnType<typeof setInterval> | null = null
let bounceOffset = 0
let legSwing = 0
let earFlop = 0
let tailSwing = 0
let tailWave = 0
let danceRotation = 0
let danceSkewX = 0
let danceSkewY = 0

const COLORS = computed(() => pet.animationColors)
const LEG_COLOR = computed(() => pet.legColor)
const FOOT_COLOR = computed(() => pet.footColor)
const EAR_INNER = computed(() => pet.earInnerColor)

let lastLegColor = 0xff7aa2
let lastFootColor = 0xd63384
let lastEarInner = 0xff9eb5

/** Soft mochi bunny ears — round teardrop shape with inner pink. */
function drawEars(cy: number, color: number, flop = 0) {
  if (!petGraphic) return

  const g = petGraphic
  const flopL = flop
  const flopR = -flop

  // Left ear (behind head — drawn before body covers the base)
  g.moveTo(-26, -6 + cy)
  g.bezierCurveTo(-58 + flopL, -18 + cy, -54 + flopL, -58 + cy, -34 + flopL * 0.6, -66 + cy)
  g.bezierCurveTo(-16 + flopL * 0.4, -62 + cy, -10, -38 + cy, -18, -18 + cy)
  g.closePath()
  g.fill(color)

  // Right ear
  g.moveTo(26, -6 + cy)
  g.bezierCurveTo(58 + flopR, -18 + cy, 54 + flopR, -58 + cy, 34 + flopR * 0.6, -66 + cy)
  g.bezierCurveTo(16 + flopR * 0.4, -62 + cy, 10, -38 + cy, 18, -18 + cy)
  g.closePath()
  g.fill(color)
}

function drawEarInner(cy: number, flop = 0) {
  if (!petGraphic) return

  const g = petGraphic
  const flopL = flop
  const flopR = -flop

  g.moveTo(-28, -16 + cy)
  g.bezierCurveTo(-40 + flopL * 0.5, -28 + cy, -38 + flopL * 0.4, -48 + cy, -28 + flopL * 0.3, -54 + cy)
  g.bezierCurveTo(-20 + flopL * 0.2, -50 + cy, -20, -32 + cy, -24, -18 + cy)
  g.closePath()
  g.fill({ color: lastEarInner, alpha: 0.92 })

  g.moveTo(28, -16 + cy)
  g.bezierCurveTo(40 + flopR * 0.5, -28 + cy, 38 + flopR * 0.4, -48 + cy, 28 + flopR * 0.3, -54 + cy)
  g.bezierCurveTo(20 + flopR * 0.2, -50 + cy, 20, -32 + cy, 24, -18 + cy)
  g.closePath()
  g.fill({ color: lastEarInner, alpha: 0.92 })
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

function danceFitScale(rotation: number): number {
  // Tail sits back-right-up in local space; track where the tip points as we spin.
  const tipAngle = rotation * 1.15 + 0.62
  const proj = Math.max(
    Math.abs(Math.cos(tipAngle)) * TAIL_EXTENT,
    Math.abs(Math.sin(tipAngle)) * TAIL_EXTENT,
    LEG_EXTENT,
  )
  const maxHalf = Math.min(CANVAS_W, CANVAS_H) / 2 - DANCE_EDGE_MARGIN
  return Math.min(DANCE_FIT_SCALE, maxHalf / (proj + DANCE_EDGE_MARGIN))
}

function applyDanceTransform(anim: Animation) {
  if (!petGraphic) return

  if (anim === 'happy') {
    const phase = danceRotation
    const fit = danceFitScale(phase)
    const tumbleX = 0.78 + Math.abs(Math.sin(phase * 0.95)) * 0.18
    const tumbleY = 0.78 + Math.abs(Math.cos(phase * 0.78)) * 0.18
    petGraphic.rotation = phase * 1.15
    petGraphic.skew.set(
      Math.sin(phase * 1.35) * 0.07 + danceSkewX,
      Math.cos(phase * 1.05) * 0.05 + danceSkewY,
    )
    petGraphic.scale.set(fit * tumbleX, fit * tumbleY)
    return
  }

  petGraphic.rotation = 0
  petGraphic.skew.set(0, 0)
  petGraphic.scale.set(pet.facing === 'left' ? -1 : 1, 1)
}

function drawPet(color: number, scale = 1, eyeOpen = true) {
  if (!petGraphic) return
  petGraphic.clear()

  const anim = pet.currentAnimation
  const cy = BODY_CY + bounceOffset
  const r = BODY_R * scale
  const legTop = cy + r - 6

  // Tail root hidden under body, then legs / ears / body, then visible upper tail
  drawTailBase(cy, color, tailSwing, tailWave)
  drawLegs(legTop)
  drawEars(cy, color, earFlop)

  petGraphic.circle(0, cy, r)
  petGraphic.fill(color)

  // Inner ear detail (on top of head)
  drawEarInner(cy, earFlop)

  // Upper tail after ears so the curl peak is not covered
  drawTailUpper(cy, color, tailSwing, tailWave)

  // Eyes
  if (eyeOpen) {
    petGraphic.circle(-18, -8 + cy, 7)
    petGraphic.circle(18, -8 + cy, 7)
    petGraphic.fill(0x333333)
    petGraphic.circle(-15, -10 + cy, 2.5)
    petGraphic.circle(21, -10 + cy, 2.5)
    petGraphic.fill(0xffffff)
  } else {
    petGraphic.moveTo(-25, -8 + cy)
    petGraphic.lineTo(-11, -8 + cy)
    petGraphic.moveTo(11, -8 + cy)
    petGraphic.lineTo(25, -8 + cy)
    petGraphic.stroke({ width: 2.5, color: 0x333333 })
  }

  // Mouth
  if (anim === 'happy' || anim === 'eat') {
    petGraphic.arc(0, 6 + cy, 13, 0, Math.PI)
    petGraphic.stroke({ width: 2.5, color: 0x333333 })
  } else if (anim === 'sad') {
    petGraphic.arc(0, 18 + cy, 9, Math.PI, 0)
    petGraphic.stroke({ width: 2.5, color: 0x333333 })
  } else {
    petGraphic.moveTo(-9, 10 + cy)
    petGraphic.lineTo(9, 10 + cy)
    petGraphic.stroke({ width: 2.5, color: 0x333333 })
  }

  // Blush
  if (anim === 'happy' || anim === 'walk') {
    petGraphic.circle(-30, 4 + cy, 9)
    petGraphic.circle(30, 4 + cy, 9)
    petGraphic.fill({ color: 0xff6b8a, alpha: 0.4 })
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
        earFlop = Math.sin(frame * 0.12) * 2
        tailSwing = Math.sin(frame * 0.1) * 6
        tailWave = Math.sin(frame * 0.16 + 1.2) * 3.5
        break
      case 'happy':
        danceRotation = frame * 0.11
        danceSkewX = Math.sin(frame * 0.17) * 0.04
        danceSkewY = Math.cos(frame * 0.13) * 0.03
        bounceOffset = Math.abs(Math.sin(frame * 0.55)) * -9
        legSwing = Math.sin(frame * 0.9) * 12
        earFlop = Math.sin(frame * 0.7) * 8
        tailSwing = Math.sin(frame * 0.65) * 16
        tailWave = Math.sin(frame * 0.85 + 0.8) * 8
        break
      case 'sad':
        bounceOffset = 3
        legSwing = 0
        earFlop = -4
        tailSwing = Math.sin(frame * 0.06) * 3 - 14
        tailWave = Math.sin(frame * 0.08) * 2
        break
      case 'sleep':
        bounceOffset = Math.sin(frame * 0.08) * 1.5
        legSwing = 0
        earFlop = 3
        tailSwing = Math.sin(frame * 0.05) * 2 - 10
        tailWave = 0
        break
      case 'eat':
        bounceOffset = Math.sin(frame * 0.5) * 2
        legSwing = Math.sin(frame * 0.5) * 5
        earFlop = Math.sin(frame * 0.45) * 3
        tailSwing = Math.sin(frame * 0.4) * 10
        tailWave = Math.sin(frame * 0.52) * 4.5
        break
      case 'walk':
        legSwing = Math.sin(frame * 0.55) * 18
        bounceOffset = Math.abs(Math.sin(frame * 0.55)) * 5
        earFlop = Math.sin(frame * 0.55) * 6
        tailSwing = Math.sin(frame * 0.55) * 16
        tailWave = Math.sin(frame * 0.72 + 0.5) * 6.5
        break
    }

    const eyeOpen =
      anim === 'sleep' ? frame % 40 > 30 : anim === 'eat' ? frame % 10 < 7 : true
    const scale =
      anim === 'happy' ? 1 + Math.sin(frame * 0.3) * 0.04
      : anim === 'walk' ? 1 + Math.sin(frame * 0.55) * 0.02
      : anim === 'sad' ? 0.96
      : 1

    drawPet(color, scale, eyeOpen)
    applyDanceTransform(anim)
  }, 50)
}

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
