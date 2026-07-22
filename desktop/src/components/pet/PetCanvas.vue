<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import * as PIXI from 'pixi.js'
import { usePetStore } from '@/stores/petStore'
import type { Animation } from '@/stores/petStore'

const pet = usePetStore()
const canvasRef = ref<HTMLCanvasElement>()

const CANVAS_W = 200
const CANVAS_H = 210
const BODY_R = 48
const BODY_CY = -18

let app: PIXI.Application | null = null
let petGraphic: PIXI.Graphics | null = null
let animTimer: ReturnType<typeof setInterval> | null = null
let bounceOffset = 0
let legSwing = 0
let earFlop = 0

const COLORS: Record<Animation, number> = {
  idle: 0xffb3c6,
  happy: 0xff8fab,
  sad: 0xadb5bd,
  sleep: 0xcdb4db,
  eat: 0xffd6a5,
  walk: 0xffcad4,
}

const LEG_COLOR = 0xff7aa2
const FOOT_COLOR = 0xd63384
const EAR_INNER = 0xff9eb5

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
  g.fill({ color: EAR_INNER, alpha: 0.92 })

  g.moveTo(28, -16 + cy)
  g.bezierCurveTo(40 + flopR * 0.5, -28 + cy, 38 + flopR * 0.4, -48 + cy, 28 + flopR * 0.3, -54 + cy)
  g.bezierCurveTo(20 + flopR * 0.2, -50 + cy, 20, -32 + cy, 24, -18 + cy)
  g.closePath()
  g.fill({ color: EAR_INNER, alpha: 0.92 })
}

function drawLegs(legTop: number) {
  if (!petGraphic) return
  const swing = legSwing
  const legW = 14
  const legH = 30

  // Left leg + foot
  petGraphic.roundRect(-20 + swing, legTop, legW, legH, 5)
  petGraphic.fill(LEG_COLOR)
  petGraphic.ellipse(-13 + swing, legTop + legH + 1, 11, 8)
  petGraphic.fill(FOOT_COLOR)

  // Right leg + foot
  petGraphic.roundRect(6 - swing, legTop, legW, legH, 5)
  petGraphic.fill(LEG_COLOR)
  petGraphic.ellipse(13 - swing, legTop + legH + 1, 11, 8)
  petGraphic.fill(FOOT_COLOR)
}

function drawPet(color: number, scale = 1, eyeOpen = true) {
  if (!petGraphic) return
  petGraphic.clear()

  const anim = pet.currentAnimation
  const cy = BODY_CY + bounceOffset
  const r = BODY_R * scale
  const legTop = cy + r - 6

  // Legs under the body (drawn first, then body covers the top of legs)
  drawLegs(legTop)

  // Ears behind head
  drawEars(cy, color, earFlop)

  // Body
  petGraphic.circle(0, cy, r)
  petGraphic.fill(color)

  // Inner ear detail (on top of head)
  drawEarInner(cy, earFlop)

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
    const color = COLORS[anim]

    switch (anim) {
      case 'idle':
        bounceOffset = Math.sin(frame * 0.15) * 2
        legSwing = Math.sin(frame * 0.14) * 4
        earFlop = Math.sin(frame * 0.12) * 2
        break
      case 'happy':
        bounceOffset = Math.abs(Math.sin(frame * 0.4)) * -6
        legSwing = Math.sin(frame * 0.4) * 7
        earFlop = Math.sin(frame * 0.35) * 5
        break
      case 'sad':
        bounceOffset = 3
        legSwing = 0
        earFlop = -4
        break
      case 'sleep':
        bounceOffset = Math.sin(frame * 0.08) * 1.5
        legSwing = 0
        earFlop = 3
        break
      case 'eat':
        bounceOffset = Math.sin(frame * 0.5) * 2
        legSwing = Math.sin(frame * 0.5) * 5
        earFlop = Math.sin(frame * 0.45) * 3
        break
      case 'walk':
        legSwing = Math.sin(frame * 0.55) * 18
        bounceOffset = Math.abs(Math.sin(frame * 0.55)) * 5
        earFlop = Math.sin(frame * 0.55) * 6
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
  }, 50)
}

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
  petGraphic.x = CANVAS_W / 2
  petGraphic.y = 92
  petGraphic.scale.x = pet.facing === 'left' ? -1 : 1
  app.stage.addChild(petGraphic)

  startAnimLoop(pet.currentAnimation)
})

watch(() => pet.currentAnimation, (anim) => {
  startAnimLoop(anim)
})

watch(() => pet.facing, () => {
  if (petGraphic) {
    petGraphic.scale.x = pet.facing === 'left' ? -1 : 1
  }
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
  width: 200px;
  height: 210px;
  display: block;
  pointer-events: none;
}
</style>
