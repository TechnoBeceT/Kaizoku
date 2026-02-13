<script setup lang="ts">
import { type DownloadInfo, QueueStatus } from '~/types'
import { getApiConfig } from '~/utils/api-config'

const props = defineProps<{
  download: DownloadInfo
  progress?: number
  showActions?: boolean
}>()

const emit = defineEmits<{
  retry: []
  delete: []
}>()

const statusLabel = computed(() => {
  switch (props.download.status) {
    case QueueStatus.RUNNING: return 'Downloading'
    case QueueStatus.COMPLETED: return 'Completed'
    case QueueStatus.WAITING: return 'Scheduled'
    case QueueStatus.FAILED: return 'Error'
    default: return 'Unknown'
  }
})

const statusColor = computed(() => {
  switch (props.download.status) {
    case QueueStatus.RUNNING: return 'info'
    case QueueStatus.COMPLETED: return 'success'
    case QueueStatus.WAITING: return 'neutral'
    case QueueStatus.FAILED: return 'error'
    default: return 'neutral'
  }
})

const scheduledTimeLabel = computed(() => {
  if (props.download.status !== QueueStatus.WAITING || !props.download.scheduledDateUTC) return null
  const scheduled = new Date(props.download.scheduledDateUTC)
  const now = new Date()
  const diffMs = scheduled.getTime() - now.getTime()

  if (diffMs <= 0) return 'Starting soon'

  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return 'Starting soon'
  if (diffMin < 60) return `in ${diffMin}m`
  const diffHr = Math.floor(diffMin / 60)
  const remainMin = diffMin % 60
  if (diffHr < 24) return remainMin > 0 ? `in ${diffHr}h ${remainMin}m` : `in ${diffHr}h`
  return scheduled.toLocaleString()
})

function formatThumbnailUrl(url?: string): string {
  if (!url) return '/kaizoku.net.png'
  if (url.startsWith('http')) return url
  const config = getApiConfig()
  return config.baseUrl ? `${config.baseUrl}/api/${url}` : `/api/${url}`
}

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}
</script>

<template>
  <UCard class="w-full">
    <div class="flex items-start gap-3">
      <img
        :src="formatThumbnailUrl(download.thumbnailUrl)"
        :alt="download.title"
        class="w-[60px] h-[80px] rounded-md object-cover shrink-0"
        @error="onImgError"
      />
      <div class="flex-1 min-w-0 space-y-1">
        <div class="flex items-center justify-between gap-2">
          <h4 class="text-sm font-medium truncate">{{ download.title }}</h4>
          <UBadge :color="statusColor" size="xs" class="shrink-0">{{ statusLabel }}</UBadge>
        </div>
        <p v-if="download.chapterTitle" class="text-xs text-muted truncate">{{ download.chapterTitle }}</p>
        <div v-if="download.provider" class="text-xs text-muted">
          {{ download.provider }}
          <span v-if="download.language"> &middot; {{ download.language }}</span>
        </div>
        <div class="flex items-center gap-2 text-xs text-muted">
          <span v-if="download.retries > 0">
            Retries: {{ download.retries }}
          </span>
          <span v-if="scheduledTimeLabel">
            {{ scheduledTimeLabel }}
          </span>
        </div>
        <div v-if="progress !== undefined" class="space-y-0.5 mt-0.5">
          <div class="flex items-center justify-between">
            <span class="text-xs text-muted">{{ Math.round(progress) }}%</span>
          </div>
          <UProgress :model-value="progress" size="sm" />
        </div>
        <div v-if="showActions" class="flex gap-1 pt-1">
          <UButton size="xs" variant="outline" icon="i-lucide-refresh-cw" label="Retry" @click="emit('retry')" />
          <UButton size="xs" variant="outline" color="error" icon="i-lucide-trash-2" label="Delete" @click="emit('delete')" />
        </div>
      </div>
    </div>
  </UCard>
</template>
