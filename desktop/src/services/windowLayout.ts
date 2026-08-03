import {
  isTauri,
  isTauriWindowReady,
  PET_W,
  PET_H,
  LOGIN_W,
  LOGIN_H,
  PET_WITH_SIDE_W,
  PET_WITH_SIDE_H,
  setWindowSize,
  setExpandedPanelLayout,
  setCompactPetLayout,
  waitForTauriWindow,
} from './chatWindow'

export { PET_W, PET_H, LOGIN_W, LOGIN_H, PET_WITH_SIDE_W, PET_WITH_SIDE_H, isTauri }

/** 等 Tauri 窗口 API 就绪后再调整 pet 壳尺寸 */
async function whenPetWindowReady(): Promise<boolean> {
  if (!isTauri()) return false
  if (!isTauriWindowReady()) {
    await waitForTauriWindow()
  }
  return isTauriWindowReady()
}

export async function setPetOnlyLayout() {
  if (!(await whenPetWindowReady())) return
  await setCompactPetLayout()
}

export async function setLoginLayout() {
  if (!(await whenPetWindowReady())) return
  await setWindowSize(LOGIN_W, LOGIN_H)
}

/** Expand pet window to fit a right-side panel (settings / onboarding). */
export async function setSidePanelLayout() {
  await setExpandedPanelLayout(true)
}

export async function syncPetShellLayout(expanded: boolean) {
  if (expanded) {
    await setExpandedPanelLayout(false)
  } else {
    await setCompactPetLayout()
  }
}
