<script setup lang="ts">
import type { PetSKU } from '../types'

defineProps<{ sku: PetSKU }>()
</script>

<template>
  <div
    class="preview"
    :style="{
      background: `linear-gradient(160deg, ${sku.skin?.colors?.idle ?? '#ffb3c6'}, ${sku.skin?.colors?.happy ?? '#ff8fab'})`,
    }"
  >
    <span
      class="ear left"
      :style="{ background: sku.skin?.colors?.ear_inner ?? '#ff9eb5' }"
    />
    <span
      class="ear right"
      :style="{ background: sku.skin?.colors?.ear_inner ?? '#ff9eb5' }"
    />
    <span class="face" />
  </div>
</template>

<style scoped>
.preview {
  width: 88px;
  height: 88px;
  border-radius: 50%;
  position: relative;
  flex-shrink: 0;
  box-shadow: 0 4px 16px rgba(255, 143, 171, 0.35);
}

.ear {
  position: absolute;
  width: 22px;
  height: 32px;
  border-radius: 50% 50% 40% 40%;
  top: 6px;
}

.ear.left {
  left: 10px;
  transform: rotate(-18deg);
}

.ear.right {
  right: 10px;
  transform: rotate(18deg);
}

.face {
  position: absolute;
  left: 50%;
  top: 54%;
  transform: translate(-50%, -50%);
  width: 26px;
  height: 4px;
  background: rgba(51, 51, 51, 0.35);
  border-radius: 2px;
}
</style>
