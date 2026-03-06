<script setup lang="ts">
import { JobType } from '~/types'

const { startWizard } = useImportWizardState()

const isUpdateAllRunning = ref(false)
const updateAllMutation = useUpdateAllSeries()
const verifyAllMutation = useVerifyAll()
const isVerifyAllRunning = ref(false)

const { getProgressForJob, isJobCompleted, isJobFailed, getJobProgress } = useSignalRProgress({
  jobTypes: [JobType.UpdateAllSeries, JobType.VerifyAll],
  onComplete: (jobType) => {
    if (jobType === JobType.UpdateAllSeries) isUpdateAllRunning.value = false
    if (jobType === JobType.VerifyAll) isVerifyAllRunning.value = false
  },
  onError: (_error, jobType) => {
    if (jobType === JobType.UpdateAllSeries) isUpdateAllRunning.value = false
    if (jobType === JobType.VerifyAll) isVerifyAllRunning.value = false
  },
})

const updateProgress = computed(() => getProgressForJob(JobType.UpdateAllSeries))
const isUpdateCompleted = computed(() => isJobCompleted(JobType.UpdateAllSeries))
const isUpdateFailed = computed(() => isJobFailed(JobType.UpdateAllSeries))
const updateProgressValue = computed(() => isUpdateCompleted.value ? 100 : (getJobProgress(JobType.UpdateAllSeries) || 0))
const showUpdateProgress = computed(() => updateProgress.value !== null || isUpdateAllRunning.value)

const verifyProgress = computed(() => getProgressForJob(JobType.VerifyAll))
const isVerifyCompleted = computed(() => isJobCompleted(JobType.VerifyAll))
const isVerifyFailed = computed(() => isJobFailed(JobType.VerifyAll))
const verifyProgressValue = computed(() => isVerifyCompleted.value ? 100 : (getJobProgress(JobType.VerifyAll) || 0))
const showVerifyProgress = computed(() => verifyProgress.value !== null || isVerifyAllRunning.value)

interface SeriesOrphanInfo {
  seriesId: string
  title: string
  orphans: string[]
}
interface VerifyResult {
  totalSeries: number
  badFiles: number
  missingFiles: number
  orphanFiles: number
  fixedCount: number
  seriesWithOrphans: SeriesOrphanInfo[]
}
const verifyResult = computed<VerifyResult | null>(() => {
  const param = verifyProgress.value?.parameter as VerifyResult | undefined
  return param?.totalSeries !== undefined ? param : null
})
const showOrphanDetails = ref(false)

async function handleUpdateAll() {
  try {
    isUpdateAllRunning.value = true
    await updateAllMutation.mutateAsync()
  } catch {
    isUpdateAllRunning.value = false
  }
}

async function handleVerifyAll() {
  try {
    isVerifyAllRunning.value = true
    await verifyAllMutation.mutateAsync()
  } catch {
    isVerifyAllRunning.value = false
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

      <!-- Verify All Series -->
      <div class="space-y-2">
        <UButton
          size="sm"
          icon="i-lucide-shield-check"
          label="Verify All Series"
          :loading="verifyAllMutation.isPending.value || isVerifyAllRunning"
          @click="handleVerifyAll"
        />
        <p class="text-sm text-muted">
          Checks all series for missing, corrupt, or duplicate files. Fixes DB records and queues re-downloads for broken chapters.
        </p>
      </div>

      <!-- Update All Progress -->
      <div v-if="showUpdateProgress" class="space-y-2">
        <UCard :class="{ 'ring-2 ring-primary': !isUpdateCompleted && !isUpdateFailed && updateProgress }">
          <div class="space-y-2">
            <div class="flex items-center gap-3">
              <UIcon
                v-if="isUpdateFailed"
                name="i-lucide-alert-circle"
                class="size-5 text-error"
              />
              <UIcon
                v-else-if="isUpdateCompleted"
                name="i-lucide-check-circle"
                class="size-5 text-primary"
              />
              <UIcon
                v-else
                name="i-lucide-loader-circle"
                class="size-5 text-primary animate-spin"
              />
              <span class="font-medium">Updating All Series</span>
              <span v-if="isUpdateFailed" class="text-sm text-error">Failed</span>
            </div>
            <UProgress :model-value="updateProgressValue" size="xs" />
            <div class="flex justify-between text-sm text-muted">
              <span>{{ updateProgress?.message || 'Processing...' }}</span>
              <span>{{ Math.round(updateProgressValue) }}%</span>
            </div>
          </div>
        </UCard>
      </div>

      <!-- Verify All Progress -->
      <div v-if="showVerifyProgress" class="space-y-2">
        <UCard :class="{ 'ring-2 ring-primary': !isVerifyCompleted && !isVerifyFailed && verifyProgress }">
          <div class="space-y-2">
            <div class="flex items-center gap-3">
              <UIcon
                v-if="isVerifyFailed"
                name="i-lucide-alert-circle"
                class="size-5 text-error"
              />
              <UIcon
                v-else-if="isVerifyCompleted"
                name="i-lucide-check-circle"
                class="size-5 text-primary"
              />
              <UIcon
                v-else
                name="i-lucide-loader-circle"
                class="size-5 text-primary animate-spin"
              />
              <span class="font-medium">Verifying All Series</span>
              <span v-if="isVerifyFailed" class="text-sm text-error">Failed</span>
            </div>
            <UProgress :model-value="verifyProgressValue" size="xs" />
            <div class="flex justify-between text-sm text-muted">
              <span>{{ verifyProgress?.message || 'Processing...' }}</span>
              <span>{{ Math.round(verifyProgressValue) }}%</span>
            </div>
          </div>
        </UCard>
      </div>

      <!-- Update Completion -->
      <div v-if="isUpdateCompleted" class="bg-success/10 border border-success/20 rounded-lg p-4">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Update All Series completed successfully!</span>
        </div>
        <p class="text-sm mt-1 text-muted">
          All series have been updated with consistent naming and metadata.
        </p>
      </div>

      <!-- Verify Completion -->
      <div v-if="isVerifyCompleted" class="bg-success/10 border border-success/20 rounded-lg p-4 space-y-3">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Verify All Series completed!</span>
        </div>
        <p class="text-sm text-muted">
          {{ verifyProgress?.message || 'All series have been verified.' }}
        </p>

        <!-- Orphan details -->
        <div v-if="verifyResult?.seriesWithOrphans?.length" class="space-y-2">
          <UButton
            size="xs"
            variant="outline"
            :icon="showOrphanDetails ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'"
            :label="`${verifyResult.seriesWithOrphans.length} series with orphan files`"
            @click="showOrphanDetails = !showOrphanDetails"
          />
          <div v-if="showOrphanDetails" class="space-y-2 max-h-64 overflow-y-auto">
            <div
              v-for="s in verifyResult.seriesWithOrphans"
              :key="s.seriesId"
              class="bg-default rounded-lg p-3 space-y-1"
            >
              <NuxtLink
                :to="`/library/series?id=${s.seriesId}`"
                class="font-medium text-sm text-primary hover:underline"
              >
                {{ s.title }}
              </NuxtLink>
              <span class="text-sm text-muted ml-2">{{ s.orphans.length }} orphan{{ s.orphans.length === 1 ? '' : 's' }}</span>
              <div class="text-xs text-muted pl-2">
                <div v-for="file in s.orphans.slice(0, 5)" :key="file">{{ file }}</div>
                <div v-if="s.orphans.length > 5" class="italic">...and {{ s.orphans.length - 5 }} more</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </UCard>
</template>
