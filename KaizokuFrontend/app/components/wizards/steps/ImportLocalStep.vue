<script setup lang="ts">
import { JobType } from '~/types'

const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
}>()

const currentActionIndex = ref(-1)
const allCompleted = ref(false)
const hasStarted = ref(false)
const completedJobs = ref(new Set<JobType>())

const scanMutation = useSetupWizardScanLocalFiles()
const installMutation = useSetupWizardInstallExtensions()
const searchMutation = useSetupWizardSearchSeries()

function handleJobComplete(jobType: JobType) {
  if (completedJobs.value.has(jobType)) return
  completedJobs.value.add(jobType)

  const triggerNext = async () => {
    try {
      if (jobType === JobType.ScanLocalFiles) {
        currentActionIndex.value = 1
        await installMutation.mutateAsync()
      } else if (jobType === JobType.InstallAdditionalExtensions) {
        currentActionIndex.value = 2
        await searchMutation.mutateAsync()
      } else if (jobType === JobType.SearchProviders) {
        allCompleted.value = true
        currentActionIndex.value = -1
      }
    } catch (err) {
      console.error(`Failed after ${jobType}:`, err)
      props.setError(`Failed to continue after action`)
      currentActionIndex.value = -1
    }
  }
  triggerNext()
}

function handleJobError(error: string) {
  props.setError(`Action failed: ${error}`)
  currentActionIndex.value = -1
}

const { connectionReady, getProgressForJob, isJobCompleted, isJobFailed, getJobProgress } = useSignalRProgress({
  jobTypes: [JobType.ScanLocalFiles, JobType.InstallAdditionalExtensions, JobType.SearchProviders],
  onComplete: handleJobComplete,
  onError: handleJobError,
})

const actions = [
  { title: 'Scan Local Files', jobType: JobType.ScanLocalFiles },
  { title: 'Install Additional Sources', jobType: JobType.InstallAdditionalExtensions },
  { title: 'Search Series', jobType: JobType.SearchProviders },
]

onMounted(async () => {
  if (!hasStarted.value) {
    hasStarted.value = true
    props.setError(null)
    currentActionIndex.value = 0
    try {
      // Wait for WebSocket connection before firing jobs —
      // otherwise fast-completing jobs (empty storage) broadcast
      // before the listener is ready and get lost.
      await connectionReady
      await scanMutation.mutateAsync()
    } catch {
      props.setError('Failed to start scan process')
      currentActionIndex.value = -1
      hasStarted.value = false
    }
  }
})

watch([currentActionIndex, allCompleted], () => {
  props.setIsLoading(currentActionIndex.value >= 0)
  props.setCanProgress(allCompleted.value)
})
</script>

<template>
  <div class="space-y-6">
    <p class="text-sm text-muted">
      This step will automatically scan your local files, install any needed sources, and search for series matches.
      All actions will run automatically.
    </p>

    <div class="max-h-[60vh] overflow-y-auto space-y-4">
      <h3 class="text-lg font-medium">Import Progress</h3>

      <UCard
        v-for="(action, index) in actions"
        :key="action.jobType"
        :class="{ 'ring-2 ring-primary': currentActionIndex === index }"
      >
        <div class="space-y-2">
          <div class="flex items-center gap-3 text-base font-semibold">
            <UIcon
              v-if="isJobFailed(action.jobType)"
              name="i-lucide-alert-circle"
              class="size-5 text-error"
            />
            <UIcon
              v-else-if="isJobCompleted(action.jobType)"
              name="i-lucide-check-circle"
              class="size-5 text-primary"
            />
            <UIcon
              v-else-if="currentActionIndex === index"
              name="i-lucide-loader-circle"
              class="size-5 text-primary animate-spin"
            />
            <div v-else class="size-5 rounded-full border-2 border-muted" />
            <span>{{ action.title }}</span>
            <span v-if="isJobFailed(action.jobType)" class="text-sm text-error font-normal">Failed</span>
          </div>
          <UProgress :model-value="isJobCompleted(action.jobType) ? 100 : getJobProgress(action.jobType)" size="xs" />
          <div class="flex justify-between text-sm text-muted">
            <span>{{ getProgressForJob(action.jobType)?.message || 'Waiting...' }}</span>
            <span>{{ Math.round(isJobCompleted(action.jobType) ? 100 : getJobProgress(action.jobType)) }}%</span>
          </div>
          <div v-if="getProgressForJob(action.jobType)?.errorMessage" class="text-sm text-error bg-error/10 p-2 rounded">
            {{ getProgressForJob(action.jobType)?.errorMessage }}
          </div>
        </div>
      </UCard>

      <div v-if="allCompleted" class="bg-success/10 border border-success/20 rounded-lg p-4">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Series process completed successfully!</span>
        </div>
        <p class="text-sm mt-1">You can now proceed to review and confirm the imported series.</p>
      </div>
    </div>
  </div>
</template>
