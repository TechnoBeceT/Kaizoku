<script setup lang="ts">
import { type DownloadInfoList, QueueStatus, ErrorDownloadAction } from '~/types'
import { downloadsService } from '~/services/downloadsService'

const props = defineProps<{
  status: QueueStatus
  title: string
}>()

const open = defineModel<boolean>('open', { default: false })

const PAGE_SIZE = 20
const page = ref(1)
const keyword = ref('')
const debouncedKeyword = useDebounce(keyword, 300)
const loading = ref(false)
const data = ref<DownloadInfoList | null>(null)
const actionLoading = ref<string | null>(null)

const offset = computed(() => (page.value - 1) * PAGE_SIZE)
const totalPages = computed(() => {
  if (!data.value) return 0
  return Math.ceil(data.value.totalCount / PAGE_SIZE)
})

async function fetchData() {
  loading.value = true
  try {
    data.value = await downloadsService.getDownloadsByStatusWithCount(
      props.status,
      PAGE_SIZE,
      debouncedKeyword.value || undefined,
      offset.value,
    )
  }
  catch {
    data.value = null
  }
  finally {
    loading.value = false
  }
}

async function handleRetry(id: string) {
  actionLoading.value = id
  try {
    await downloadsService.manageErrorDownload(id, ErrorDownloadAction.Retry)
    await fetchData()
  }
  finally {
    actionLoading.value = null
  }
}

async function handleDelete(id: string) {
  actionLoading.value = id
  try {
    await downloadsService.manageErrorDownload(id, ErrorDownloadAction.Delete)
    await fetchData()
  }
  finally {
    actionLoading.value = null
  }
}

watch(open, (val) => {
  if (val) {
    page.value = 1
    keyword.value = ''
    fetchData()
  }
})

watch([debouncedKeyword], () => {
  page.value = 1
  fetchData()
})

watch(page, () => fetchData())
</script>

<template>
  <UModal v-model:open="open" :ui="{ content: 'sm:max-w-4xl max-h-[90vh]' }">
    <template #header>
      <div class="flex items-center justify-between w-full">
        <div class="flex items-center gap-2">
          <h3 class="text-lg font-semibold">{{ title }}</h3>
          <UBadge v-if="data">{{ data.totalCount }}</UBadge>
        </div>
      </div>
    </template>

    <template #body>
      <div class="space-y-4">
        <UInput
          v-model="keyword"
          icon="i-lucide-search"
          placeholder="Search by title, provider, language..."
          class="w-full"
        />

        <div v-if="loading" class="flex items-center justify-center py-12">
          <UIcon name="i-lucide-loader-2" class="size-8 animate-spin text-muted" />
        </div>

        <div v-else-if="!data?.downloads?.length" class="flex items-center justify-center py-12 text-muted">
          <p>No downloads found</p>
        </div>

        <div v-else class="space-y-2">
          <div class="grid gap-2 md:grid-cols-2 lg:grid-cols-3">
            <QueueDownloadCard
              v-for="dl in data.downloads"
              :key="dl.id"
              :download="dl"
              :show-actions="status === QueueStatus.FAILED"
              @retry="handleRetry(dl.id)"
              @delete="handleDelete(dl.id)"
            />
          </div>

          <div v-if="totalPages > 1" class="flex items-center justify-center gap-2 pt-4">
            <UButton
              size="xs"
              variant="outline"
              icon="i-lucide-chevron-left"
              :disabled="page <= 1"
              @click="page--"
            />
            <span class="text-sm text-muted">
              Page {{ page }} of {{ totalPages }}
            </span>
            <UButton
              size="xs"
              variant="outline"
              icon="i-lucide-chevron-right"
              :disabled="page >= totalPages"
              @click="page++"
            />
          </div>
        </div>
      </div>
    </template>
  </UModal>
</template>
