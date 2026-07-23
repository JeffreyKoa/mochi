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
}

export interface AdoptResult {
  message: string
  pet: { id: number; name: string; sku_id: string }
  sku: PetSKU
  order: { id: number; status: string }
}
