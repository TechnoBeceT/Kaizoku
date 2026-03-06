<script setup lang="ts">
import { JobType } from '~/types'

const { data: jobStatus } = useJobStatus()

const kindLabels: Record<string, string> = {
  get_chapters: 'Refresh Chapters',
  get_latest: 'Fetch Latest',
  update_extensions: 'Update Extensions',
  update_all_series: 'Update All Series',
  daily_update: 'Daily Update',
  scan_local_files: 'Scan Local Files',
  install_extensions: 'Install Extensions',
  search_providers: 'Search Providers',
  refresh_all_chapters: 'Refresh All Chapters',
  refresh_all_latest: 'Refresh All Latest',
  import_series: 'Import Series',
  verify_all_series: 'Verify All Series',
  upgrade_all_sources: 'Upgrade All Sources',
}

const activeKinds = computed(() => {
  if (!jobStatus.value?.kinds) return []
  return jobStatus.value.kinds
    .filter(k => k.running > 0 || k.available > 0 || k.scheduled > 0 || k.pending > 0)
    .sort((a, b) => (b.running + b.available) - (a.running + a.available))
})

const recentlyCompleted = computed(() => {
  if (!jobStatus.value?.kinds) return []
  return jobStatus.value.kinds
    .filter(k => k.completed > 0 && k.running === 0 && k.available === 0 && k.scheduled === 0 && k.pending === 0)
})

const totalRunning = computed(() => jobStatus.value?.kinds.reduce((s, k) => s + k.running, 0) ?? 0)
const totalQueued = computed(() => jobStatus.value?.kinds.reduce((s, k) => s + k.available + k.scheduled + k.pending, 0) ?? 0)
const hasActivity = computed(() => totalRunning.value > 0 || totalQueued.value > 0 || recentlyCompleted.value.length > 0)

const { startWizard } = useImportWizardState()

const isUpdateAllRunning = ref(false)
const updateAllMutation = useUpdateAllSeries()
const verifyAllMutation = useVerifyAll()
const isVerifyAllRunning = ref(false)
const upgradeAllMutation = useUpgradeAllSources()
const isUpgradeAllRunning = ref(false)

const { getProgressForJob, isJobCompleted, isJobFailed, getJobProgress, resetJob } = useSignalRProgress({
  jobTypes: [JobType.UpdateAllSeries, JobType.VerifyAll, JobType.UpgradeAllSources],
  onComplete: (jobType) => {
    if (jobType === JobType.UpdateAllSeries) isUpdateAllRunning.value = false
    if (jobType === JobType.VerifyAll) isVerifyAllRunning.value = false
    if (jobType === JobType.UpgradeAllSources) isUpgradeAllRunning.value = false
  },
  onError: (_error, jobType) => {
    if (jobType === JobType.UpdateAllSeries) isUpdateAllRunning.value = false
    if (jobType === JobType.VerifyAll) isVerifyAllRunning.value = false
    if (jobType === JobType.UpgradeAllSources) isUpgradeAllRunning.value = false
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
  refreshQueued: number
  seriesWithOrphans: SeriesOrphanInfo[]
}
const verifyResult = computed<VerifyResult | null>(() => {
  const param = verifyProgress.value?.parameter as VerifyResult | undefined
  return param?.totalSeries !== undefined ? param : null
})
const upgradeProgress = computed(() => getProgressForJob(JobType.UpgradeAllSources))
const isUpgradeCompleted = computed(() => isJobCompleted(JobType.UpgradeAllSources))
const isUpgradeFailed = computed(() => isJobFailed(JobType.UpgradeAllSources))
const upgradeProgressValue = computed(() => isUpgradeCompleted.value ? 100 : (getJobProgress(JobType.UpgradeAllSources) || 0))
const showUpgradeProgress = computed(() => upgradeProgress.value !== null || isUpgradeAllRunning.value)

interface UpgradeResult {
  totalSeries: number
  totalUpgraded: number
  totalSkipped: number
}
const upgradeResult = computed<UpgradeResult | null>(() => {
  const param = upgradeProgress.value?.parameter as UpgradeResult | undefined
  return param?.totalSeries !== undefined ? param : null
})

const showOrphanDetails = ref(false)

async function handleUpgradeAll() {
  try {
    resetJob(JobType.UpgradeAllSources)
    isUpgradeAllRunning.value = true
    await upgradeAllMutation.mutateAsync()
  } catch {
    isUpgradeAllRunning.value = false
  }
}

async function handleUpdateAll() {
  try {
    resetJob(JobType.UpdateAllSeries)
    isUpdateAllRunning.value = true
    await updateAllMutation.mutateAsync()
  } catch {
    isUpdateAllRunning.value = false
  }
}

async function handleVerifyAll() {
  try {
    resetJob(JobType.VerifyAll)
    isVerifyAllRunning.value = true
    showOrphanDetails.value = false
    await verifyAllMutation.mutateAsync()
  } catch {
    isVerifyAllRunning.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <!-- Background Tasks -->
    <UCard v-if="hasActivity">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">Background Tasks</span>
          <div class="flex items-center gap-2 text-sm text-muted">
            <span v-if="totalRunning > 0">{{ totalRunning }} running</span>
            <span v-if="totalRunning > 0 && totalQueued > 0">&middot;</span>
            <span v-if="totalQueued > 0">{{ totalQueued }} queued</span>
          </div>
        </div>
      </template>

      <div class="space-y-3">
        <div v-for="kind in activeKinds" :key="kind.kind" class="space-y-1">
          <div class="flex items-center justify-between text-sm">
            <span class="font-medium">{{ kindLabels[kind.kind] || kind.kind }}</span>
            <span class="text-muted">
              {{ kind.completed }}/{{ kind.total }}
              <span v-if="kind.running > 0">({{ kind.running }} active)</span>
            </span>
          </div>
          <UProgress :model-value="kind.total > 0 ? (kind.completed / kind.total) * 100 : 0" size="xs" />
        </div>

        <div v-for="kind in recentlyCompleted" :key="kind.kind" class="flex items-center justify-between text-sm">
          <span class="font-medium">{{ kindLabels[kind.kind] || kind.kind }}</span>
          <div class="flex items-center gap-1 text-primary">
            <UIcon name="i-lucide-check-circle" class="size-4" />
            <span>{{ kind.completed }} completed</span>
          </div>
        </div>
      </div>
    </UCard>

    <!-- Jobs -->
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

      <!-- Upgrade All Sources -->
      <div class="space-y-2">
        <UButton
          size="sm"
          icon="i-lucide-arrow-up-circle"
          label="Upgrade All Sources"
          :loading="upgradeAllMutation.isPending.value || isUpgradeAllRunning"
          @click="handleUpgradeAll"
        />
        <p class="text-sm text-muted">
          Re-downloads chapters from better sources when available. Replaces files from lower-priority sources with the best available source. Skips sources that have permanently failed.
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

      <!-- Upgrade All Progress -->
      <div v-if="showUpgradeProgress" class="space-y-2">
        <UCard :class="{ 'ring-2 ring-primary': !isUpgradeCompleted && !isUpgradeFailed && upgradeProgress }">
          <div class="space-y-2">
            <div class="flex items-center gap-3">
              <UIcon
                v-if="isUpgradeFailed"
                name="i-lucide-alert-circle"
                class="size-5 text-error"
              />
              <UIcon
                v-else-if="isUpgradeCompleted"
                name="i-lucide-check-circle"
                class="size-5 text-primary"
              />
              <UIcon
                v-else
                name="i-lucide-loader-circle"
                class="size-5 text-primary animate-spin"
              />
              <span class="font-medium">Upgrading All Sources</span>
              <span v-if="isUpgradeFailed" class="text-sm text-error">Failed</span>
            </div>
            <UProgress :model-value="upgradeProgressValue" size="xs" />
            <div class="flex justify-between text-sm text-muted">
              <span>{{ upgradeProgress?.message || 'Processing...' }}</span>
              <span>{{ Math.round(upgradeProgressValue) }}%</span>
            </div>
          </div>
        </UCard>
      </div>

      <!-- Upgrade Completion -->
      <div v-if="isUpgradeCompleted" class="bg-success/10 border border-success/20 rounded-lg p-4">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-check-circle" class="size-5 text-primary" />
          <span class="font-medium">Upgrade All Sources completed!</span>
        </div>
        <p class="text-sm mt-1 text-muted">
          {{ upgradeProgress?.message || 'All sources have been checked.' }}
        </p>
        <p v-if="upgradeResult" class="text-sm text-muted mt-1">
          {{ upgradeResult.totalUpgraded }} chapters queued for upgrade across {{ upgradeResult.totalSeries - upgradeResult.totalSkipped }} series.
        </p>
      </div>
    </div>
  </UCard>
  </div>
</template>
