import type { Animation } from '@/stores/petStore'

/** Map backend animation names to pet canvas animations. */
export function mapServerAnimation(raw: string | undefined): Animation {
  if (!raw) return 'idle'
  switch (raw) {
    case 'happy':
    case 'walk':
    case 'eat':
    case 'sleep':
    case 'sad':
    case 'idle':
      return raw
    case 'concerned':
      return 'sad'
    default:
      return 'idle'
  }
}
