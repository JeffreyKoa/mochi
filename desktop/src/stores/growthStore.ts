import { defineStore } from 'pinia'
import { ref } from 'vue'
import { completeOnboarding, getBond, getBrief, getMemories, deleteMemory, approveBriefEntry, rejectBriefEntry } from '@/services/api'
import type { BondProfile, MemoryItem, OnboardingInput, UserBrief, UserBriefEntry } from '@/types/growth'
import { markOnboardingDone, needsOnboarding } from '@/types/growth'

export const useGrowthStore = defineStore('growth', () => {
  const bond = ref<BondProfile | null>(null)
  const brief = ref<UserBrief | null>(null)
  const briefEntries = ref<UserBriefEntry[]>([])
  const pendingBriefEntries = ref<UserBriefEntry[]>([])
  const writeApproval = ref(false)
  const memories = ref<MemoryItem[]>([])
  const loading = ref(false)
  const showSettings = ref(false)
  const onboardingRequired = ref(false)
  const petId = ref(0)

  async function fetchBondAndBrief() {
    loading.value = true
    try {
      const [bondData, briefData] = await Promise.all([getBond(), getBrief()])
      bond.value = bondData as unknown as BondProfile
      petId.value = bond.value.pet_id
      brief.value = (briefData.brief as UserBrief) ?? null
      briefEntries.value = (briefData.entries as UserBriefEntry[]) ?? []
      pendingBriefEntries.value = (briefData.pending_entries as UserBriefEntry[]) ?? []
      writeApproval.value = !!briefData.write_approval
      onboardingRequired.value = needsOnboarding(bond.value, petId.value)
    } finally {
      loading.value = false
    }
  }

  async function fetchMemories() {
    const list = await getMemories()
    memories.value = (list ?? []) as typeof memories.value
  }

  async function removeMemory(id: number) {
    await deleteMemory(id)
    memories.value = memories.value.filter((m) => m.id !== id)
  }

  async function submitOnboarding(input: OnboardingInput) {
    await completeOnboarding(input)
    markOnboardingDone(petId.value || bond.value?.pet_id || 0)
    await fetchBondAndBrief()
    onboardingRequired.value = false
  }

  function openSettings() {
    showSettings.value = true
    void fetchBondAndBrief()
    void fetchMemories()
  }

  function closeSettings() {
    showSettings.value = false
  }

  function skipOnboarding() {
    const id = petId.value || bond.value?.pet_id
    if (id) markOnboardingDone(id)
    onboardingRequired.value = false
  }

  async function approvePendingEntry(id: number) {
    await approveBriefEntry(id)
    await fetchBondAndBrief()
  }

  async function rejectPendingEntry(id: number) {
    await rejectBriefEntry(id)
    pendingBriefEntries.value = pendingBriefEntries.value.filter((e) => e.id !== id)
  }

  return {
    bond,
    brief,
    briefEntries,
    pendingBriefEntries,
    writeApproval,
    memories,
    loading,
    showSettings,
    onboardingRequired,
    petId,
    fetchBondAndBrief,
    fetchMemories,
    removeMemory,
    approvePendingEntry,
    rejectPendingEntry,
    submitOnboarding,
    skipOnboarding,
    openSettings,
    closeSettings,
  }
})
