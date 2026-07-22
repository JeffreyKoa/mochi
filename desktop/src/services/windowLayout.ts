import { PhysicalSize } from '@tauri-apps/api/dpi'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { isTauri, PET_W, PET_H, LOGIN_W, LOGIN_H } from './chatWindow'

export { PET_W, PET_H, LOGIN_W, LOGIN_H, isTauri }

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
