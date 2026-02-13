<script setup lang="ts">
import { QueueStatus, type DownloadInfo, ErrorDownloadAction } from '~/types'

const { debouncedSearchTerm } = useSearchState()
const { data: settings } = useSettings()
const { downloads: activeDownloads, downloadCount } = useDownloadProgress()

const limit = computed(() => settings.value?.numberOfSimultaneousDownloads || 10)

const { data: completedData } = useCompletedDownloadsWithCount(limit, computed(() => debouncedSearchTerm.value.trim() || undefined))
const { data: scheduledData } = useWaitingDownloadsWithCount(limit, computed(() => debouncedSearchTerm.value.trim() || undefined))
const { data: failedData } = useFailedDownloadsWithCount(limit, computed(() => debouncedSearchTerm.value.trim() || undefined))
const manageError = useManageErrorDownload()

const showCompletedModal = ref(false)
const showScheduledModal = ref(false)
const showFailedModal = ref(false)

function handleRetry(id: string) {
  manageError.mutate({ id, action: ErrorDownloadAction.Retry })
}

function handleDeleteError(id: string) {
  manageError.mutate({ id, action: ErrorDownloadAction.Delete })
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- Active Downloads -->
    <UCard>
      <template #header>
        <div class="flex items-center gap-2">
          <span class="font-semibold">Active Downloads</span>
          <UBadge v-if="downloadCount > 0">{{ downloadCount }}</UBadge>
        </div>
      </template>
      <div v-if="activeDownloads.length === 0" class="flex items-center justify-center py-8 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-download" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No active downloads</p>
        </div>
      </div>
      <div v-else class="grid gap-2 md:grid-cols-3 lg:grid-cols-5">
        <QueueDownloadCard
          v-for="dl in activeDownloads"
          :key="dl.id"
          :download="{ id: dl.id, title: dl.cardInfo.title, provider: dl.cardInfo.provider, language: dl.cardInfo.language, status: 1, scheduledDateUTC: '', retries: 0, chapterTitle: dl.cardInfo.chapterName, thumbnailUrl: dl.cardInfo.thumbnailUrl }"
          :progress="dl.percentage"
        />
      </div>
    </UCard>

    <!-- Completed Downloads -->
    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="font-semibold">Latest Downloads</span>
            <UBadge>{{ completedData?.totalCount || 0 }}</UBadge>
          </div>
          <UButton
            v-if="completedData && completedData.totalCount > (completedData.downloads?.length || 0)"
            size="xs"
            variant="outline"
            label="View All"
            icon="i-lucide-expand"
            @click="showCompletedModal = true"
          />
        </div>
      </template>
      <div v-if="!completedData?.downloads?.length" class="flex items-center justify-center py-8 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-check-circle" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No completed downloads</p>
        </div>
      </div>
      <div v-else class="grid gap-2 md:grid-cols-3 lg:grid-cols-5">
        <QueueDownloadCard v-for="dl in completedData.downloads" :key="dl.id" :download="dl" />
      </div>
    </UCard>

    <!-- Scheduled Downloads -->
    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="font-semibold">Scheduled Downloads</span>
            <UBadge>{{ scheduledData?.totalCount || 0 }}</UBadge>
          </div>
          <UButton
            v-if="scheduledData && scheduledData.totalCount > (scheduledData.downloads?.length || 0)"
            size="xs"
            variant="outline"
            label="View All"
            icon="i-lucide-expand"
            @click="showScheduledModal = true"
          />
        </div>
      </template>
      <div v-if="!scheduledData?.downloads?.length" class="flex items-center justify-center py-8 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-clock" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No scheduled downloads</p>
        </div>
      </div>
      <div v-else class="grid gap-2 md:grid-cols-3 lg:grid-cols-5">
        <QueueDownloadCard v-for="dl in scheduledData.downloads" :key="dl.id" :download="dl" />
      </div>
    </UCard>

    <!-- Error Downloads -->
    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="font-semibold">Error Downloads</span>
            <UBadge>{{ failedData?.totalCount || 0 }}</UBadge>
          </div>
          <UButton
            v-if="failedData && failedData.totalCount > (failedData.downloads?.length || 0)"
            size="xs"
            variant="outline"
            label="View All"
            icon="i-lucide-expand"
            @click="showFailedModal = true"
          />
        </div>
      </template>
      <div v-if="!failedData?.downloads?.length" class="flex items-center justify-center py-8 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-smile" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No failed downloads</p>
        </div>
      </div>
      <div v-else class="grid gap-2 md:grid-cols-3 lg:grid-cols-5">
        <QueueDownloadCard
          v-for="dl in failedData.downloads"
          :key="dl.id"
          :download="dl"
          show-actions
          @retry="handleRetry(dl.id)"
          @delete="handleDeleteError(dl.id)"
        />
      </div>
    </UCard>

    <!-- Jobs Panel -->
    <JobsPanel />

    <!-- Paginated Modals -->
    <QueueDownloadsListModal
      v-model:open="showCompletedModal"
      :status="QueueStatus.COMPLETED"
      title="All Completed Downloads"
    />
    <QueueDownloadsListModal
      v-model:open="showScheduledModal"
      :status="QueueStatus.WAITING"
      title="All Scheduled Downloads"
    />
    <QueueDownloadsListModal
      v-model:open="showFailedModal"
      :status="QueueStatus.FAILED"
      title="All Failed Downloads"
    />
  </div>
</template>
