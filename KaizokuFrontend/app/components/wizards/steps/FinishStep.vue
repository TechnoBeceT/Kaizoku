<script setup lang="ts">
import { JobType } from '~/types'

const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
  disableDownloads: boolean
}>()

const importMutation = useSetupWizardImportSeries()
const importCompleted = ref(false)
const hasTriggered = ref(false)

const { getProgressForJob, isJobCompleted, isJobFailed, getJobProgress } = useSignalRProgress({
  jobTypes: [JobType.ImportSeries],
  onComplete: () => { importCompleted.value = true },
  onError: (error) => { props.setError(`Import failed: ${error}`) },
})

const progressData = computed(() => getProgressForJob(JobType.ImportSeries))
const progress = computed(() => importCompleted.value || isJobCompleted(JobType.ImportSeries) ? 100 : getJobProgress(JobType.ImportSeries))
const isFailed = computed(() => isJobFailed(JobType.ImportSeries))
const isActive = computed(() => hasTriggered.value && !importCompleted.value && !isJobCompleted(JobType.ImportSeries) && !isFailed.value)

onMounted(async () => {
  if (!hasTriggered.value) {
    hasTriggered.value = true
    props.setError(null)
    try {
      await importMutation.mutateAsync(props.disableDownloads)
    } catch (err) {
      props.setError('Failed to start import process')
      hasTriggered.value = false
    }
  }
})

watch([importCompleted, () => isJobCompleted(JobType.ImportSeries), () => isJobFailed(JobType.ImportSeries)], () => {
  const isImporting = hasTriggered.value && !importCompleted.value && !isJobCompleted(JobType.ImportSeries) && !isJobFailed(JobType.ImportSeries)
  props.setIsLoading(isImporting)
  props.setCanProgress(importCompleted.value || isJobCompleted(JobType.ImportSeries))
})
</script>

<template>
  <div class="space-y-6">
    <p class="text-sm text-muted">
      Final step: Importing your selected series into the library.
      Please wait while the process completes.
    </p>

    <div class="max-h-[60vh] overflow-y-auto space-y-6">
      <UCard :class="{ 'ring-2 ring-primary': isActive }">
        <div class="space-y-4">
          <div class="flex items-center gap-3">
            <UIcon
              v-if="isFailed"
              name="i-lucide-alert-circle"
              class="size-6 text-error"
            />
            <UIcon
              v-else-if="importCompleted || isJobCompleted(JobType.ImportSeries)"
              name="i-lucide-check-circle"
              class="size-6 text-primary"
            />
            <UIcon
              v-else-if="isActive"
              name="i-lucide-loader-circle"
              class="size-6 text-primary animate-spin"
            />
            <UIcon
              v-else
              name="i-lucide-flag"
              class="size-6 text-muted"
            />
            <span class="font-semibold">Series Import</span>
          </div>
          <UProgress :model-value="progress" size="sm" />
          <div class="flex justify-between text-sm">
            <span class="text-muted">
              <template v-if="isFailed">Import process failed</template>
              <template v-else-if="importCompleted || isJobCompleted(JobType.ImportSeries)">Import process completed successfully!</template>
              <template v-else-if="isActive">Importing series...</template>
              <template v-else>Preparing to import series...</template>
            </span>
            <span class="text-muted">{{ Math.round(progress) }}%</span>
          </div>
          <p class="text-sm text-muted">
            <template v-if="importCompleted || isJobCompleted(JobType.ImportSeries)">All selected series have been imported into your library.</template>
            <template v-else-if="isActive && progressData?.message">{{ progressData.message }}</template>
            <template v-else-if="isFailed && progressData?.errorMessage">{{ progressData.errorMessage }}</template>
            <template v-else>This may take a few minutes depending on the number of series being imported.</template>
          </p>
          <div v-if="progressData?.errorMessage" class="text-sm text-error bg-error/10 p-3 rounded">
            <strong>Error:</strong> {{ progressData.errorMessage }}
          </div>
        </div>
      </UCard>

      <div v-if="importCompleted || isJobCompleted(JobType.ImportSeries)" class="bg-success/10 border border-success/20 rounded-lg p-4">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Import process completed successfully!</span>
        </div>
        <p class="text-sm mt-1">Congratulations! Your import wizard is now complete.</p>
      </div>

      <div v-if="isFailed" class="bg-error/10 border border-error/20 rounded-lg p-6 text-center">
        <UIcon name="i-lucide-alert-circle" class="size-12 text-error mx-auto mb-4" />
        <h3 class="text-lg font-semibold mb-2">Import Failed</h3>
        <p class="text-sm mb-4">The import process encountered an error. You can try again or skip this step.</p>
      </div>
    </div>
  </div>
</template>
