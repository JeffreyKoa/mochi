import { PhysicalSize } from '@tauri-apps/api/dpi'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { isTauri, PET_W, PET_H, LOGIN_W, LOGIN_H, PET_WITH_SIDE_W, PET_WITH_SIDE_H } from './chatWindow'

export { PET_W, PET_H, LOGIN_W, LOGIN_H, PET_WITH_SIDE_W, PET_WITH_SIDE_H, isTauri }

export async function setWindowSize(width: number, height: number) {
  if (!isTauri()) return
  await getCurrentWindow().setSize(new PhysicalSize(width, height))
}

export async function setPetOnlyLayout() {
  await setWindowSize(PET_W, PET_H)
}

export async function setLoginLayout() {
  await setWindowSize(LOGIN_W, LOGIN_H)
}

/** Expand pet window to fit a right-side panel (onboarding / relationship). */
export async function setSidePanelLayout() {
  await setWindowSize(PET_WITH_SIDE_W, PET_WITH_SIDE_H)
}
