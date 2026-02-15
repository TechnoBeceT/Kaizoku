<script setup lang="ts">
import draggable from 'vuedraggable'
import { type SeriesExtendedInfo, type ProviderExtendedInfo, SeriesStatus, QueueStatus } from '~/types'
import { getStatusDisplay } from '~/utils/series-status'
import { getCountryCodeForLanguage } from '~/utils/language-country-map'
import { getApiConfig } from '~/utils/api-config'

const route = useRoute()
const router = useRouter()
const { setSeriesTitle } = useSeriesState()

const seriesId = computed(() => route.query.id as string)
const { data: series, isLoading } = useSeriesById(seriesId)
const { data: downloads } = useDownloadsForSeries(seriesId)
const updateMutation = useUpdateSeries()
const deleteMutation = useDeleteSeries()
const verifyMutation = useVerifyIntegrity()
const deepVerifyMutation = useDeepVerify()
const cleanupMutation = useCleanupSeries()

const showDeleteDialog = ref(false)
const deletePhysical = ref(false)
const showDeleteProviderDialog = ref(false)
const deleteProviderPhysical = ref(false)
const providerToDelete = ref<ProviderExtendedInfo | null>(null)
const matchingProviderId = ref<string | null>(null)
const toast = useToast()

// Local copy of providers for drag-and-drop reordering
const localProviders = ref<ProviderExtendedInfo[]>([])

watch(() => series.value?.providers, (providers) => {
  if (providers) {
    localProviders.value = providers.map(p => ({ ...p })).sort((a, b) => a.importance - b.importance)
  }
}, { immediate: true })

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}

function formatThumbnailUrl(url?: string): string {
  if (!url) return '/kaizoku.net.png'
  if (url.startsWith('http')) return url
  const config = getApiConfig()
  return config.baseUrl ? `${config.baseUrl}/api/${url}` : `/api/${url}`
}

watch(() => series.value?.title, (title) => {
  if (title) setSeriesTitle(title)
}, { immediate: true })

// Computed display values based on active providers
const activeProviderForTitle = computed(() =>
  localProviders.value.find(p => !p.isDeleted && p.useTitle)
)
const activeProviderForCover = computed(() =>
  localProviders.value.find(p => !p.isDeleted && p.useCover)
)
const displayTitle = computed(() =>
  activeProviderForTitle.value?.title || series.value?.title || ''
)
const displayThumbnail = computed(() =>
  activeProviderForCover.value?.thumbnailUrl || series.value?.thumbnailUrl
)

// Effective status
const knownProviders = computed(() =>
  localProviders.value.filter(p => !p.isUnknown && !p.isUninstalled && !p.isDeleted)
)
const hasNonUnknownProviders = computed(() =>
  localProviders.value.some(p => !p.isUnknown && !p.isDeleted)
)
const hasActiveProviders = computed(() =>
  knownProviders.value.some(p => !p.isDisabled)
)
const effectiveStatus = computed(() => {
  if (series.value && !hasActiveProviders.value) return SeriesStatus.DISABLED
  // Use status from the highest-importance active provider (lowest importance number)
  const activeByImportance = knownProviders.value
    .filter(p => !p.isDisabled && p.status !== SeriesStatus.UNKNOWN)
    .sort((a, b) => a.importance - b.importance)
  if (activeByImportance.length > 0) return activeByImportance[0].status
  return series.value?.status ?? SeriesStatus.UNKNOWN
})

// Sorted downloads: by status (running first, then waiting, completed, failed), then by chapter number ascending
const sortedDownloads = computed(() => {
  if (!downloads.value?.length) return []
  const statusOrder: Record<number, number> = {
    [QueueStatus.RUNNING]: 0,
    [QueueStatus.WAITING]: 1,
    [QueueStatus.COMPLETED]: 2,
    [QueueStatus.FAILED]: 3,
  }
  return [...downloads.value].sort((a, b) => {
    const sa = statusOrder[a.status] ?? 9
    const sb = statusOrder[b.status] ?? 9
    if (sa !== sb) return sa - sb
    return (a.chapter ?? 0) - (b.chapter ?? 0)
  })
})

async function handleDelete() {
  if (!series.value) return
  try {
    await deleteMutation.mutateAsync({ id: series.value.id, alsoPhysical: deletePhysical.value })
    toast.add({ title: 'Series deleted', color: 'success' })
    router.push('/library')
  } catch {
    toast.add({ title: 'Failed to delete series', color: 'error' })
  }
  showDeleteDialog.value = false
}

async function togglePause() {
  if (!series.value) return
  const updated = { ...series.value, pausedDownloads: !series.value.pausedDownloads }
  await updateMutation.mutateAsync(updated as SeriesExtendedInfo)
}

async function handleVerify() {
  if (!series.value) return
  const result = await verifyMutation.mutateAsync(series.value.id)
  if (result.success && result.missingFiles === 0 && result.orphanFiles.length === 0) {
    toast.add({ title: 'All files are valid', color: 'success' })
  } else {
    const parts: string[] = []
    if (result.badFiles.length > 0) parts.push(`${result.badFiles.length} bad files`)
    if (result.missingFiles > 0) parts.push(`${result.missingFiles} missing files`)
    if (result.orphanFiles.length > 0) parts.push(`${result.orphanFiles.length} orphan files`)
    if (result.fixedCount > 0) parts.push(`${result.fixedCount} records fixed`)
    if (result.redownloadQueued > 0) parts.push(`${result.redownloadQueued} re-downloads queued`)
    toast.add({ title: `Verify: ${parts.join(', ')}`, color: result.fixedCount > 0 ? 'warning' : 'info' })
  }
}

async function handleDeepVerify() {
  if (!series.value) return
  const result = await deepVerifyMutation.mutateAsync(series.value.id)
  if (result.success) {
    toast.add({ title: 'Deep verify: no issues found', color: 'success' })
  } else {
    const parts: string[] = []
    if (result.suspiciousFiles.length > 0) parts.push(`${result.suspiciousFiles.length} suspicious files`)
    if (result.sourceIssues.length > 0) parts.push(`${result.sourceIssues.length} source issues`)
    toast.add({ title: `Deep verify: ${parts.join(', ')}`, color: 'warning' })
  }
}

async function handleCleanup() {
  if (!series.value) return
  await cleanupMutation.mutateAsync(series.value.id)
  toast.add({ title: 'Cleanup complete', color: 'success' })
}

async function onDragEnd() {
  if (!series.value) return
  localProviders.value.forEach((p, i) => {
    p.importance = i
  })
  try {
    const updated = { ...series.value, providers: localProviders.value }
    await updateMutation.mutateAsync(updated as SeriesExtendedInfo)
  } catch {
    // Revert to server state on failure
    if (series.value?.providers) {
      localProviders.value = series.value.providers.map(p => ({ ...p })).sort((a, b) => a.importance - b.importance)
    }
    toast.add({ title: 'Failed to save provider order', color: 'error' })
  }
}

async function setExclusive(provider: ProviderExtendedInfo, field: 'useCover' | 'useTitle') {
  if (!series.value) return
  const updatedProviders = localProviders.value.map((p) => ({
    ...p,
    [field]: p.id === provider.id,
  }))
  localProviders.value = updatedProviders
  const updated = { ...series.value, providers: updatedProviders }
  await updateMutation.mutateAsync(updated as SeriesExtendedInfo)
}

async function toggleDisabled(provider: ProviderExtendedInfo) {
  if (!series.value) return
  const updatedProviders = localProviders.value.map((p) => {
    if (p.id === provider.id) {
      return { ...p, isDisabled: !p.isDisabled }
    }
    return p
  })
  localProviders.value = updatedProviders
  const updated = { ...series.value, providers: updatedProviders }
  await updateMutation.mutateAsync(updated as SeriesExtendedInfo)
}

function confirmDeleteProvider(provider: ProviderExtendedInfo) {
  providerToDelete.value = provider
  deleteProviderPhysical.value = false
  showDeleteProviderDialog.value = true
}

async function handleDeleteProvider() {
  if (!series.value || !providerToDelete.value) return
  const provider = providerToDelete.value
  const deleteFiles = deleteProviderPhysical.value
  try {
    const updatedProviders = localProviders.value.map((p) => {
      if (p.id === provider.id) {
        return { ...p, isDeleted: true, isDisabled: true, deleteFiles }
      }
      return p
    })
    localProviders.value = updatedProviders.filter(p => !p.isDeleted)
    const updated = { ...series.value, providers: updatedProviders }
    await updateMutation.mutateAsync(updated as SeriesExtendedInfo)
    toast.add({ title: 'Source deleted', color: 'success' })
  } catch {
    toast.add({ title: 'Failed to delete source', color: 'error' })
  }
  showDeleteProviderDialog.value = false
  providerToDelete.value = null
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return ''
  const utcStr = dateStr.includes('Z') || dateStr.includes('+') ? dateStr : dateStr + 'Z'
  const d = new Date(utcStr)
  return `${d.toLocaleDateString()} ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`
}
</script>

<template>
  <div>
    <div v-if="isLoading" class="flex items-center justify-center min-h-[400px]">
      <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin" />
    </div>

    <div v-else-if="series" class="grid grid-cols-1 lg:grid-cols-5 gap-3">
      <!-- Left Column (4/5 width) -->
      <div class="lg:col-span-4 space-y-3">
        <!-- Series Header Card -->
        <UCard>
          <div class="flex gap-4">
            <!-- Cover Image -->
            <div class="shrink-0">
              <img
                :src="formatThumbnailUrl(displayThumbnail)"
                :alt="displayTitle"
                class="h-96 rounded-lg object-cover border"
                style="aspect-ratio: 4/6"
                @error="onImgError"
              />
            </div>

            <!-- Series Info -->
            <div class="flex-1 flex flex-col gap-2 relative min-w-0">
              <!-- Status Badge (top right) -->
              <div class="absolute top-0 right-0">
                <UBadge :color="getStatusDisplay(effectiveStatus).color as any" size="lg">
                  {{ getStatusDisplay(effectiveStatus).text }}
                </UBadge>
              </div>

              <!-- Title & Chapter Info -->
              <div>
                <h1 class="text-2xl font-bold pr-24">{{ displayTitle }}</h1>
                <div class="flex items-center gap-3 mt-2 text-sm">
                  <UBadge v-if="series.chapterList" variant="subtle">{{ series.chapterList }}</UBadge>
                  <span v-if="series.lastChapter" class="text-muted">
                    Last: <UBadge variant="subtle">{{ series.lastChapter }}</UBadge>
                  </span>
                  <span v-if="series.lastChangeUTC" class="text-sm font-medium">
                    {{ formatDate(series.lastChangeUTC) }}
                  </span>
                </div>
              </div>

              <!-- Author / Artist -->
              <div v-if="series.author || series.artist" class="grid grid-cols-2 gap-2 text-sm">
                <div v-if="series.author">
                  <span class="text-muted">Author:</span>
                  <span class="ml-2 font-medium">{{ series.author }}</span>
                </div>
                <div v-if="series.artist">
                  <span class="text-muted">Artist:</span>
                  <span class="ml-2 font-medium">{{ series.artist }}</span>
                </div>
              </div>

              <!-- Genres -->
              <div v-if="series.genre?.length" class="flex flex-wrap gap-1">
                <UBadge v-for="genre in series.genre" :key="genre" variant="subtle" size="xs">{{ genre }}</UBadge>
              </div>

              <!-- Description (flexible area) -->
              <div v-if="series.description" class="flex-1">
                <p class="text-sm">{{ series.description }}</p>
              </div>

              <!-- Series Path -->
              <div v-if="series.path" class="mt-auto">
                <div class="bg-default border border-default rounded-md px-3 py-2 text-sm font-mono text-muted break-all inline-block">
                  {{ series.path }}
                </div>
              </div>

              <!-- Action Buttons (bottom right) -->
              <div class="absolute bottom-0 right-0 flex gap-2">
                <UButton
                  icon="i-lucide-trash-2"
                  label="Delete Series"
                  color="error"
                  variant="solid"
                  size="sm"
                  @click="showDeleteDialog = true"
                />
                <UButton
                  icon="i-lucide-shield-check"
                  :label="verifyMutation.isPending.value ? 'Verifying...' : 'Verify'"
                  size="sm"
                  :loading="verifyMutation.isPending.value"
                  @click="handleVerify"
                />
                <UButton
                  icon="i-lucide-scan-search"
                  :label="deepVerifyMutation.isPending.value ? 'Deep Verifying...' : 'Deep Verify'"
                  size="sm"
                  :loading="deepVerifyMutation.isPending.value"
                  @click="handleDeepVerify"
                />
                <UButton
                  :icon="series.pausedDownloads ? 'i-lucide-play' : 'i-lucide-pause'"
                  :label="series.pausedDownloads ? 'Resume Downloads' : 'Pause Downloads'"
                  :color="series.pausedDownloads ? 'primary' : 'error'"
                  variant="solid"
                  size="sm"
                  @click="togglePause"
                />
              </div>
            </div>
          </div>
        </UCard>

        <!-- Sources Card -->
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="font-semibold">Sources</span>
                <UBadge>{{ localProviders.filter(p => !p.isDeleted).length }}</UBadge>
              </div>
              <SeriesAddSeries
                :title="series.title"
                :series-id="series.id"
                :existing-sources="localProviders.filter(p => !p.isDeleted).map(p => ({ provider: p.provider, scanlator: p.scanlator || '', lang: p.lang || '' }))"
              >
                <template #default="{ open }">
                  <UButton size="xs" icon="i-lucide-plus" label="Add Source" @click="open" />
                </template>
              </SeriesAddSeries>
            </div>
          </template>

          <draggable
            v-model="localProviders"
            item-key="id"
            handle=".drag-handle"
            ghost-class="opacity-30"
            @end="onDragEnd"
          >
            <template #item="{ element: provider, index }">
              <div
                v-if="!provider.isDeleted"
                class="p-3 mb-2 rounded-lg border border-default bg-default transition-colors"
                :class="{ 'opacity-60': provider.isDisabled }"
              >
                <div class="flex items-start gap-3">
                  <!-- Drag Handle + Importance Badge -->
                  <div class="flex flex-col items-center gap-2 shrink-0 pt-1">
                    <div class="drag-handle cursor-grab active:cursor-grabbing text-muted hover:text-default">
                      <UIcon name="i-lucide-grip-vertical" class="size-5" />
                    </div>
                    <UBadge :color="index === 0 ? 'primary' : 'neutral'" variant="solid" class="w-8 justify-center">
                      #{{ index + 1 }}
                    </UBadge>
                  </div>

                  <!-- Provider Thumbnail -->
                  <div class="shrink-0">
                    <img
                      :src="formatThumbnailUrl(provider.thumbnailUrl)"
                      :alt="provider.title"
                      class="h-56 rounded-md object-cover border"
                      style="aspect-ratio: 4/6"
                      @error="onImgError"
                    />
                  </div>

                  <!-- Provider Info -->
                  <div class="flex-1 min-w-0 space-y-2">
                    <!-- Title Row -->
                    <div class="flex items-start justify-between gap-2">
                      <div class="min-w-0">
                        <h3 class="text-lg font-semibold truncate">{{ provider.title }}</h3>
                        <!-- Provider + Scanlator + Language + Status -->
                        <div class="flex items-center gap-2 text-sm text-muted mt-0.5">
                          <a
                            v-if="provider.url"
                            :href="provider.url"
                            target="_blank"
                            rel="noopener noreferrer"
                            class="flex items-center gap-1 hover:text-default transition-colors"
                          >
                            <UIcon name="i-lucide-external-link" class="size-3.5" />
                            <span>{{ provider.provider }}</span>
                            <span v-if="provider.scanlator && provider.scanlator !== provider.provider"> &middot; {{ provider.scanlator }}</span>
                          </a>
                          <span v-else>
                            {{ provider.provider }}
                            <span v-if="provider.scanlator && provider.scanlator !== provider.provider"> &middot; {{ provider.scanlator }}</span>
                          </span>
                          <span class="text-xs px-1.5 py-0.5 rounded bg-muted font-medium">{{ provider.lang?.toUpperCase() }}</span>
                          <UBadge :color="getStatusDisplay(provider.status).color as any" size="xs">
                            {{ getStatusDisplay(provider.status).text }}
                          </UBadge>
                        </div>
                      </div>

                      <!-- Action buttons (top right) -->
                      <div class="flex gap-2 shrink-0">
                        <UButton
                          v-if="provider.isUnknown && hasNonUnknownProviders"
                          icon="i-lucide-link"
                          label="Match"
                          color="primary"
                          size="xs"
                          @click="matchingProviderId = provider.id"
                        />
                        <UButton
                          icon="i-lucide-trash-2"
                          label="Delete"
                          color="error"
                          size="xs"
                          @click="confirmDeleteProvider(provider)"
                        />
                        <UButton
                          v-if="!provider.isUnknown && !provider.isUninstalled"
                          :icon="provider.isDisabled ? 'i-lucide-power' : 'i-lucide-power-off'"
                          :label="provider.isDisabled ? 'Enable' : 'Disable'"
                          :color="provider.isDisabled ? 'primary' : 'error'"
                          size="xs"
                          @click="toggleDisabled(provider)"
                        />
                      </div>
                    </div>

                    <!-- Chapter List + Last Chapter -->
                    <div class="flex flex-wrap items-center gap-2 text-sm">
                      <UBadge v-if="provider.chapterList" variant="subtle">{{ provider.chapterList }}</UBadge>
                      <span v-if="provider.lastChapter" class="text-muted">
                        Last: <UBadge variant="subtle">{{ provider.lastChapter }}</UBadge>
                      </span>
                      <span v-if="provider.lastChangeUTC" class="text-sm font-medium">
                        {{ formatDate(provider.lastChangeUTC) }}
                      </span>
                    </div>

                    <!-- Author / Artist -->
                    <div v-if="provider.author || provider.artist" class="grid grid-cols-2 gap-2 text-sm">
                      <div v-if="provider.author">
                        <span class="text-muted">Author:</span>
                        <span class="ml-2 font-medium">{{ provider.author }}</span>
                      </div>
                      <div v-if="provider.artist">
                        <span class="text-muted">Artist:</span>
                        <span class="ml-2 font-medium">{{ provider.artist }}</span>
                      </div>
                    </div>

                    <!-- Genres -->
                    <div v-if="provider.genre?.length" class="flex flex-wrap gap-1">
                      <UBadge v-for="genre in provider.genre" :key="genre" variant="subtle" size="xs">{{ genre }}</UBadge>
                    </div>

                    <!-- Description -->
                    <p v-if="provider.description" class="text-sm line-clamp-4">{{ provider.description }}</p>

                    <!-- Controls: Cover, Title, Disabled switches -->
                    <div v-if="!provider.isUnknown" class="grid grid-cols-3 gap-4 pt-1">
                      <label class="flex items-center gap-2 text-sm cursor-pointer" @click.stop>
                        <input
                          type="radio"
                          name="detailCoverSource"
                          :checked="provider.useCover"
                          class="accent-primary"
                          :disabled="provider.isDisabled"
                          @change="setExclusive(provider, 'useCover')"
                        />
                        Use Cover
                      </label>
                      <label class="flex items-center gap-2 text-sm cursor-pointer" @click.stop>
                        <input
                          type="radio"
                          name="detailTitleSource"
                          :checked="provider.useTitle"
                          class="accent-primary"
                          :disabled="provider.isDisabled"
                          @change="setExclusive(provider, 'useTitle')"
                        />
                        Use Title
                      </label>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </draggable>
        </UCard>
      </div>

      <!-- Right Column: Downloads Panel (1/5 width) -->
      <div class="lg:col-span-1">
        <UCard class="sticky top-4">
          <template #header>
            <div class="flex items-center gap-2">
              <UIcon name="i-lucide-download" class="size-5" />
              <span class="font-semibold">Latest Downloads</span>
              <UBadge v-if="sortedDownloads.length > 0" size="xs">{{ sortedDownloads.length }}</UBadge>
            </div>
          </template>

          <div v-if="sortedDownloads.length === 0" class="flex items-center justify-center py-8 text-muted">
            <div class="text-center">
              <UIcon name="i-lucide-download" class="size-12 mx-auto mb-4 opacity-50" />
              <p>No downloads yet</p>
            </div>
          </div>
          <div v-else class="space-y-2 max-h-[calc(100vh-10rem)] overflow-y-auto">
            <QueueDownloadCard v-for="dl in sortedDownloads" :key="dl.id" :download="dl" />
          </div>
        </UCard>
      </div>
    </div>

    <!-- Delete Dialog -->
    <UModal v-model:open="showDeleteDialog">
      <template #body>
        <div class="space-y-4 p-4">
          <h3 class="text-lg font-semibold">Delete Series</h3>
          <p class="text-sm text-muted">Are you sure you want to delete "{{ series?.title }}"?</p>
          <div class="flex items-center gap-2">
            <UCheckbox v-model="deletePhysical" label="Also delete physical files" />
          </div>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" label="Cancel" @click="showDeleteDialog = false" />
            <UButton color="error" label="Delete" :loading="deleteMutation.isPending.value" @click="handleDelete" />
          </div>
        </div>
      </template>
    </UModal>

    <!-- Delete Provider Dialog -->
    <UModal v-model:open="showDeleteProviderDialog">
      <template #body>
        <div class="space-y-4 p-4">
          <h3 class="text-lg font-semibold">Delete Source</h3>
          <p class="text-sm text-muted">
            Are you sure you want to delete "{{ providerToDelete?.provider }}" ({{ providerToDelete?.lang }})?
          </p>
          <div class="flex items-center gap-2">
            <UCheckbox v-model="deleteProviderPhysical" label="Also delete downloaded chapter files" />
          </div>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" label="Cancel" @click="showDeleteProviderDialog = false" />
            <UButton color="error" label="Delete" :loading="updateMutation.isPending.value" @click="handleDeleteProvider" />
          </div>
        </div>
      </template>
    </UModal>

    <!-- Provider Match Dialog -->
    <SeriesProviderMatchDialog
      v-if="matchingProviderId"
      :provider-id="matchingProviderId"
      @close="matchingProviderId = null"
      @matched="matchingProviderId = null"
    />
  </div>
</template>
