<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query'
import type { Settings } from '~/types'

const { isWizardActive, currentStep, totalSteps, nextStep, previousStep, completeWizard } = useSetupWizardState()
const queryClient = useQueryClient()
const updateSettings = useUpdateSettings()
const isLoading = ref(false)
const error = ref<string | null>(null)
const canProgress = ref(false)
const disableDownloads = ref(false)

const isFirstStep = computed(() => currentStep.value === 0)
const isLastStep = computed(() => currentStep.value === totalSteps.value - 1)

const steps = [
  { label: 'Preferences', description: 'Configure settings', icon: 'i-lucide-sliders' },
  { label: 'Add Sources', description: 'Install sources', icon: 'i-lucide-settings' },
  { label: 'Import Local Files', description: 'Scan archives', icon: 'i-lucide-file' },
  { label: 'Confirm Imports', description: 'Review series', icon: 'i-lucide-check-square' },
  { label: 'Schedule Summary', description: 'Check incoming updates', icon: 'i-lucide-clock' },
  { label: 'Finish', description: 'Complete Import', icon: 'i-lucide-flag' },
]

async function handleNext() {
  if (isLastStep.value) {
    completeWizard()
    return
  }

  // When leaving preferences step, flush settings save and wait for backend
  // to sync repos to Suwayomi before sources are fetched on the next step
  if (currentStep.value === 0) {
    const currentSettings = queryClient.getQueryData<Settings>(['settings'])
    if (currentSettings) {
      isLoading.value = true
      error.value = null
      try {
        await updateSettings.mutateAsync(currentSettings)
      } catch {
        error.value = 'Failed to save settings. Please try again.'
        isLoading.value = false
        return
      }
      isLoading.value = false
    }
  }

  nextStep()
}

function setError_(e: string | null) { error.value = e }
function setIsLoading_(l: boolean) { isLoading.value = l }
function setCanProgress_(c: boolean) { canProgress.value = c }
</script>

<template>
  <UModal v-if="isWizardActive" :open="true" :close="false" :dismissible="false" :ui="{ content: 'sm:max-w-5xl max-h-[90vh]', body: 'p-0' }">
    <template #body>
      <div class="flex flex-col max-h-[85vh]">
        <!-- Fixed Header -->
        <div class="shrink-0 space-y-3 p-4 pb-2">
          <div>
            <h2 class="text-lg font-semibold">Setup Wizard</h2>
            <p class="text-sm text-muted">
              Configure your Kaizoku installation by following these steps to set up preferences, add sources, and import existing series.
            </p>
          </div>

          <!-- Step Indicators -->
          <div class="flex items-center gap-2 overflow-x-auto pb-1">
            <div
              v-for="(step, idx) in steps"
              :key="idx"
              class="flex items-center gap-1 text-xs whitespace-nowrap"
              :class="idx === currentStep ? 'text-primary font-medium' : idx < currentStep ? 'text-success' : 'text-muted'"
            >
              <UIcon :name="step.icon" class="size-4" />
              <span>{{ step.label }}</span>
              <UIcon v-if="idx < steps.length - 1" name="i-lucide-chevron-right" class="size-3 text-muted" />
            </div>
          </div>

          <!-- Error -->
          <div v-if="error" class="rounded-lg border bg-error/10 p-2 text-sm text-error">
            {{ error }}
          </div>
        </div>

        <!-- Scrollable Step Content -->
        <div class="flex-1 overflow-y-auto px-4 min-h-0">
          <WizardsStepsPreferencesStep
            v-if="currentStep === 0"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsAddProvidersStep
            v-if="currentStep === 1"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsImportLocalStep
            v-if="currentStep === 2"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsConfirmImportsStep
            v-if="currentStep === 3"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsScheduleUpdatesStep
            v-if="currentStep === 4"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
            @download-option-change="disableDownloads = $event"
          />
          <WizardsStepsFinishStep
            v-if="currentStep === 5"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
            :disable-downloads="disableDownloads"
          />
        </div>

        <!-- Fixed Footer -->
        <div class="shrink-0 flex justify-between items-center p-4 pt-3 border-t border-muted/20">
          <UButton
            v-if="!isFirstStep"
            variant="outline"
            icon="i-lucide-arrow-left"
            label="Previous"
            :disabled="isLoading"
            @click="previousStep"
          />
          <div v-else />
          <span class="text-sm text-muted">Step {{ currentStep + 1 }} of {{ totalSteps }}</span>
          <UButton
            :icon="isLastStep ? 'i-lucide-check' : 'i-lucide-arrow-right'"
            :label="isLastStep ? 'Finish' : 'Next'"
            :disabled="!canProgress || isLoading"
            :loading="isLoading"
            @click="handleNext"
          />
        </div>
      </div>
    </template>
  </UModal>
</template>
