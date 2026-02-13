import { useQueryClient } from '@tanstack/vue-query'

const IMPORT_WIZARD_STORAGE_KEY = 'import-wizard-state'
const TOTAL_STEPS = 4 // import, confirm, schedule, finish

interface ImportWizardState {
  isActive: boolean
  currentStep: number
  completedSteps: number
  stepData: Record<number, unknown>
}

export function useImportWizardState() {
  const queryClient = useQueryClient()

  const wizardState = useState<ImportWizardState>('kzk-import-wizard', () => ({
    isActive: false,
    currentStep: 0,
    completedSteps: 0,
    stepData: {},
  }))

  const initialized = useState('kzk-import-wizard-init', () => false)

  // Initialize from localStorage — only resume if wizard was in progress
  if (import.meta.client && !initialized.value) {
    initialized.value = true
    const saved = localStorage.getItem(IMPORT_WIZARD_STORAGE_KEY)
    if (saved) {
      try {
        const parsed = JSON.parse(saved) as ImportWizardState
        if (parsed.isActive) {
          wizardState.value = parsed
        } else {
          localStorage.removeItem(IMPORT_WIZARD_STORAGE_KEY)
        }
      } catch {
        localStorage.removeItem(IMPORT_WIZARD_STORAGE_KEY)
      }
    }
  }

  // Persist to localStorage only while wizard is active
  watch(wizardState, (state) => {
    if (import.meta.client) {
      if (state.isActive) {
        localStorage.setItem(IMPORT_WIZARD_STORAGE_KEY, JSON.stringify(state))
      } else {
        localStorage.removeItem(IMPORT_WIZARD_STORAGE_KEY)
      }
    }
  }, { deep: true })

  const isWizardActive = computed(() => wizardState.value.isActive)
  const currentStep = computed(() => wizardState.value.currentStep)
  const totalSteps = computed(() => TOTAL_STEPS)

  function startWizard() {
    wizardState.value = {
      isActive: true,
      currentStep: 0,
      completedSteps: 0,
      stepData: {},
    }
  }

  function cancelWizard() {
    wizardState.value = {
      isActive: false,
      currentStep: 0,
      completedSteps: 0,
      stepData: {},
    }
    if (import.meta.client) {
      localStorage.removeItem(IMPORT_WIZARD_STORAGE_KEY)
    }
  }

  function nextStep() {
    if (wizardState.value.currentStep >= TOTAL_STEPS - 1) return
    const nextIdx = wizardState.value.currentStep + 1

    wizardState.value = {
      ...wizardState.value,
      currentStep: nextIdx,
      completedSteps: Math.max(wizardState.value.completedSteps, nextIdx),
    }
  }

  function previousStep() {
    if (wizardState.value.currentStep <= 0) return
    const prevIdx = wizardState.value.currentStep - 1

    wizardState.value = {
      ...wizardState.value,
      currentStep: prevIdx,
    }
  }

  function completeWizard() {
    wizardState.value = {
      isActive: false,
      currentStep: 0,
      completedSteps: 0,
      stepData: {},
    }
    if (import.meta.client) {
      localStorage.removeItem(IMPORT_WIZARD_STORAGE_KEY)
    }
    // Refresh library data
    queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
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
    startWizard,
    cancelWizard,
    nextStep,
    previousStep,
    completeWizard,
    setStepData,
    getStepData,
  }
}
