import {
  isTauri,
  PET_W,
  PET_H,
  LOGIN_W,
  LOGIN_H,
  PET_WITH_SIDE_W,
  PET_WITH_SIDE_H,
  setWindowSize,
  setExpandedPanelLayout,
  setCompactPetLayout,
} from './chatWindow'

export { PET_W, PET_H, LOGIN_W, LOGIN_H, PET_WITH_SIDE_W, PET_WITH_SIDE_H, isTauri }

export async function setPetOnlyLayout() {
  await setCompactPetLayout()
}

export async function setLoginLayout() {
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
