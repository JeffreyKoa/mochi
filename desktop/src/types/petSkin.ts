import type { Animation } from '@/stores/petStore'

export interface PetSkinColors {
  idle: string
  happy: string
  sad: string
  sleep: string
  eat: string
  walk: string
  leg: string
  foot: string
  ear_inner: string
}

export interface PetSkin {
  shape: string
  colors: PetSkinColors
}

export interface PetSKU {
  sku_id: string
  name: string
  species: string
  breed: string
  breed_name: string
  tier: string
  max_age_years: number
  price_cny: number
  tagline: string
  skin: PetSkin
  personality_preset?: { traits?: string; speech_style?: string }
}

export const DEFAULT_SKIN_COLORS: PetSkinColors = {
  idle: '#ffb3c6',
  happy: '#ff8fab',
  sad: '#adb5bd',
  sleep: '#cdb4db',
  eat: '#ffd6a5',
  walk: '#ffcad4',
  leg: '#ff7aa2',
  foot: '#d63384',
  ear_inner: '#ff9eb5',
}

export function hexToPixi(hex: string): number {
  return parseInt(hex.replace('#', ''), 16)
}

export function skinColorsToPixi(colors: PetSkinColors): Record<Animation, number> {
  return {
    idle: hexToPixi(colors.idle),
    happy: hexToPixi(colors.happy),
    sad: hexToPixi(colors.sad),
    sleep: hexToPixi(colors.sleep),
    eat: hexToPixi(colors.eat),
    walk: hexToPixi(colors.walk),
  }
}

export function parsePetSkin(raw: unknown): PetSkin | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as PetSkin
  if (!o.colors?.idle) return null
  return o
}
