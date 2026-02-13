<script setup lang="ts">
const { isWizardActive, currentStep, totalSteps, nextStep, previousStep, completeWizard, cancelWizard } = useImportWizardState()
const isLoading = ref(false)
const error = ref<string | null>(null)
const canProgress = ref(false)
const disableDownloads = ref(false)

const isFirstStep = computed(() => currentStep.value === 0)
const isLastStep = computed(() => currentStep.value === totalSteps.value - 1)

const steps = [
  { label: 'Import Local Files', icon: 'i-lucide-file' },
  { label: 'Confirm Imports', icon: 'i-lucide-check-square' },
  { label: 'Schedule Updates', icon: 'i-lucide-clock' },
  { label: 'Finish', icon: 'i-lucide-flag' },
]

function handleNext() {
  if (isLastStep.value) {
    completeWizard()
  } else {
    nextStep()
  }
}

function setError_(e: string | null) { error.value = e }
function setIsLoading_(l: boolean) { isLoading.value = l }
function setCanProgress_(c: boolean) { canProgress.value = c }
</script>

<template>
  <UModal v-if="isWizardActive" :open="true" :ui="{ content: 'sm:max-w-5xl max-h-[90vh]', body: 'p-0' }" @close="cancelWizard">
    <template #body>
      <div class="flex flex-col max-h-[85vh]">
        <!-- Fixed Header -->
        <div class="shrink-0 space-y-3 p-4 pb-2">
          <div>
            <h2 class="text-lg font-semibold">Import Wizard</h2>
            <p class="text-sm text-muted">
              Import existing manga series from your local files and configure automatic updates.
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
          <WizardsStepsImportLocalStep
            v-if="currentStep === 0"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsConfirmImportsStep
            v-if="currentStep === 1"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
          />
          <WizardsStepsScheduleUpdatesStep
            v-if="currentStep === 2"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
            @download-option-change="disableDownloads = $event"
          />
          <WizardsStepsFinishStep
            v-if="currentStep === 3"
            :set-error="setError_"
            :set-is-loading="setIsLoading_"
            :set-can-progress="setCanProgress_"
            :disable-downloads="disableDownloads"
          />
        </div>

        <!-- Fixed Footer -->
        <div class="shrink-0 flex justify-between items-center p-4 pt-3 border-t border-muted/20">
          <div class="flex gap-2">
            <UButton
              variant="ghost"
              label="Cancel"
              @click="cancelWizard"
            />
            <UButton
              v-if="!isFirstStep"
              variant="outline"
              icon="i-lucide-arrow-left"
              label="Previous"
              :disabled="isLoading"
              @click="previousStep"
            />
          </div>
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
