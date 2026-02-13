<script setup lang="ts">
import { JobType } from '~/types'

const { startWizard } = useImportWizardState()

const isUpdateAllRunning = ref(false)
const updateAllMutation = useUpdateAllSeries()
const { getProgressForJob, isJobCompleted, isJobFailed, getJobProgress } = useSignalRProgress({
  jobTypes: [JobType.UpdateAllSeries],
  onComplete: () => { isUpdateAllRunning.value = false },
  onError: () => { isUpdateAllRunning.value = false },
})

const updateProgress = computed(() => getProgressForJob(JobType.UpdateAllSeries))
const isCompleted = computed(() => isJobCompleted(JobType.UpdateAllSeries))
const isFailed = computed(() => isJobFailed(JobType.UpdateAllSeries))
const progressValue = computed(() => isCompleted.value ? 100 : (getJobProgress(JobType.UpdateAllSeries) || 0))
const showProgress = computed(() => updateProgress.value !== null || isUpdateAllRunning.value)

async function handleUpdateAll() {
  try {
    isUpdateAllRunning.value = true
    await updateAllMutation.mutateAsync()
  } catch {
    isUpdateAllRunning.value = false
  }
}
</script>

<template>
  <UCard>
    <template #header>
      <span class="font-semibold">Jobs</span>
    </template>

    <div class="space-y-4">
      <!-- Import Series -->
      <div class="space-y-2">
        <UButton size="sm" icon="i-lucide-download" label="Import Series" @click="startWizard" />
        <p class="text-sm text-muted">
          Import Additional Series, or fix existing ones. This is an interactive process, and will open a wizard.
        </p>
      </div>

      <!-- Update All Series -->
      <div class="space-y-2">
        <UButton
          size="sm"
          icon="i-lucide-file-text"
          label="Update All Series"
          :loading="updateAllMutation.isPending.value || isUpdateAllRunning"
          @click="handleUpdateAll"
        />
        <p class="text-sm text-muted">
          Applies the selected title to the entire series. This process updates titles for consistency, rewrites the ComicInfo.xml, and sets the series cover to the one you selected.
        </p>
      </div>

      <!-- Progress -->
      <div v-if="showProgress" class="space-y-2">
        <UCard :class="{ 'ring-2 ring-primary': !isCompleted && !isFailed && updateProgress }">
          <div class="space-y-2">
            <div class="flex items-center gap-3">
              <UIcon
                v-if="isFailed"
                name="i-lucide-alert-circle"
                class="size-5 text-error"
              />
              <UIcon
                v-else-if="isCompleted"
                name="i-lucide-check-circle"
                class="size-5 text-primary"
              />
              <UIcon
                v-else
                name="i-lucide-loader-circle"
                class="size-5 text-primary animate-spin"
              />
              <span class="font-medium">Updating All Series</span>
              <span v-if="isFailed" class="text-sm text-error">Failed</span>
            </div>
            <UProgress :model-value="progressValue" size="xs" />
            <div class="flex justify-between text-sm text-muted">
              <span>{{ updateProgress?.message || 'Processing...' }}</span>
              <span>{{ Math.round(progressValue) }}%</span>
            </div>
          </div>
        </UCard>
      </div>

      <!-- Completion -->
      <div v-if="isCompleted" class="bg-success/10 border border-success/20 rounded-lg p-4">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Update All Series completed successfully!</span>
        </div>
        <p class="text-sm mt-1 text-muted">
          All series have been updated with consistent naming and metadata.
        </p>
      </div>
    </div>
  </UCard>
</template>
