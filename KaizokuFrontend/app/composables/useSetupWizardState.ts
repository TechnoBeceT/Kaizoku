import type { Settings } from '~/types'
import { useQueryClient } from '@tanstack/vue-query'

const WIZARD_STORAGE_KEY = 'setup-wizard-state'
const TOTAL_STEPS = 6

interface WizardState {
  isActive: boolean
  currentStep: number
  completedSteps: number
  stepData: Record<number, unknown>
}

export function useSetupWizardState() {
  const queryClient = useQueryClient()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()

  const wizardState = useState<WizardState>('kzk-setup-wizard', () => ({
    isActive: false,
    currentStep: 0,
    completedSteps: 0,
    stepData: {},
  }))

  // Track whether we've already initialized from localStorage or backend.
  // Once initialized, local state is the source of truth — the settings
  // watcher must NOT override local navigation (matching original .NET behavior).
  const initialized = useState('kzk-setup-wizard-init', () => false)
  const settingsSynced = useState('kzk-setup-wizard-settings-synced', () => false)

  // Restore from localStorage on first load — but ONLY if the wizard was
  // in progress (isActive === true).  A stale { isActive: false } entry
  // must not prevent the backend check, otherwise a fresh backend (rebuild /
  // clean DB) will never trigger the wizard.
  if (import.meta.client && !initialized.value) {
    initialized.value = true
    const saved = localStorage.getItem(WIZARD_STORAGE_KEY)
    if (saved) {
      try {
        const parsed = JSON.parse(saved) as WizardState
        if (parsed.isActive) {
          wizardState.value = parsed
          settingsSynced.value = true // Resume in-progress wizard, skip backend init
        } else {
          // Wizard was completed or inactive — clear stale entry and let backend decide
          localStorage.removeItem(WIZARD_STORAGE_KEY)
        }
      } catch {
        localStorage.removeItem(WIZARD_STORAGE_KEY)
      }
    }
  }

  // Initialize from backend settings ONLY ONCE (when no localStorage state exists).
  // After initialization, local navigation takes precedence.
  watch(settings, (s) => {
    if (!s || settingsSynced.value) return
    settingsSynced.value = true

    if (s.isWizardSetupComplete) return // Wizard already done

    const step = Math.min(s.wizardSetupStepCompleted, TOTAL_STEPS - 1)
    wizardState.value = {
      ...wizardState.value,
      isActive: true,
      currentStep: step,
      completedSteps: step,
    }
  }, { immediate: true })

  // Persist to localStorage (only while wizard is active)
  watch(wizardState, (state) => {
    if (import.meta.client) {
      if (state.isActive) {
        localStorage.setItem(WIZARD_STORAGE_KEY, JSON.stringify(state))
      } else {
        localStorage.removeItem(WIZARD_STORAGE_KEY)
      }
    }
  }, { deep: true })

  const isWizardActive = computed(() => wizardState.value.isActive)
  const currentStep = computed(() => wizardState.value.currentStep)
  const totalSteps = computed(() => TOTAL_STEPS)

  function nextStep() {
    if (wizardState.value.currentStep >= TOTAL_STEPS - 1) return
    const nextIdx = wizardState.value.currentStep + 1

    wizardState.value = {
      ...wizardState.value,
      currentStep: nextIdx,
      completedSteps: Math.max(wizardState.value.completedSteps, nextIdx),
    }

    // Save step progress to backend
    if (settings.value) {
      updateSettings.mutate({
        ...settings.value,
        wizardSetupStepCompleted: nextIdx,
      })
    }
  }

  function previousStep() {
    if (wizardState.value.currentStep <= 0) return
    const prevIdx = wizardState.value.currentStep - 1

    wizardState.value = {
      ...wizardState.value,
      currentStep: prevIdx,
    }

    if (settings.value) {
      updateSettings.mutate({
        ...settings.value,
        wizardSetupStepCompleted: prevIdx,
      })
    }
  }

  function completeWizard() {
    if (!settings.value) return

    const updated: Settings = {
      ...settings.value,
      isWizardSetupComplete: true,
      wizardSetupStepCompleted: 0,
    }

    updateSettings.mutate(updated, {
      onSettled: () => {
        wizardState.value = {
          isActive: false,
          currentStep: 0,
          completedSteps: 0,
          stepData: {},
        }
        if (import.meta.client) {
          localStorage.removeItem(WIZARD_STORAGE_KEY)
        }
        queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
      },
    })
  }

  function setStepData(stepIndex: number, data: unknown) {
    wizardState.value = {
      ...wizardState.value,
      stepData: {
        ...wizardState.value.stepData,
        [stepIndex]: data,
      },
    }
  }

  function getStepData(stepIndex: number): unknown {
    return wizardState.value.stepData[stepIndex]
  }

  return {
    isWizardActive,
    currentStep,
    totalSteps,
    nextStep,
    previousStep,
    completeWizard,
    setStepData,
    getStepData,
  }
}
