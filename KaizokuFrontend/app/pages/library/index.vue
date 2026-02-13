<script setup lang="ts">
import { SeriesStatus, type SeriesInfo } from '~/types'

const { debouncedSearchTerm } = useSearchState()
const { data: library } = useLibrary()

const tab = useSessionStorage('tab', 'all')
const selectedGenre = useSessionStorage<string | null>('genre', null)
const selectedProvider = useSessionStorage<string | null>('provider', null)
const orderBy = useSessionStorage('orderBy', 'title')
const cardWidth = useSessionStorage('cardWidth', 'w-45')

const cardWidthOptions = [
  { value: 'w-20', label: 'XS' },
  { value: 'w-32', label: 'S' },
  { value: 'w-45', label: 'M' },
  { value: 'w-58', label: 'L' },
  { value: 'w-70', label: 'XL' },
]

const deduplicatedLibrary = computed(() => {
  if (!library.value) return []
  const seen = new Set<string>()
  return library.value.filter((s) => {
    if (seen.has(s.id)) return false
    seen.add(s.id)
    return true
  })
})

const genres = computed(() => {
  const genreSet = new Set<string>()
  deduplicatedLibrary.value.forEach(s => s.genre?.forEach(g => genreSet.add(g)))
  return Array.from(genreSet).sort()
})

const providers = computed(() => {
  const provSet = new Set<string>()
  deduplicatedLibrary.value.forEach(s => s.providers?.forEach(p => provSet.add(p.provider)))
  return Array.from(provSet).sort()
})

function matchesTab(series: SeriesInfo): boolean {
  switch (tab.value) {
    case 'completed': return series.status === SeriesStatus.COMPLETED || series.status === SeriesStatus.PUBLISHING_FINISHED
    case 'active': return series.status !== SeriesStatus.COMPLETED && series.status !== SeriesStatus.PUBLISHING_FINISHED && series.isActive && !series.pausedDownloads
    case 'paused': return series.pausedDownloads
    case 'unassigned': return series.hasUnknown === true
    default: return true
  }
}

function baseFilter(series: SeriesInfo): boolean {
  if (selectedGenre.value && !series.genre?.includes(selectedGenre.value)) return false
  if (selectedProvider.value && !series.providers?.some(p => p.provider === selectedProvider.value)) return false
  return true
}

const filteredLibrary = computed(() => {
  let result = deduplicatedLibrary.value
  if (debouncedSearchTerm.value.trim()) {
    const term = debouncedSearchTerm.value.toLowerCase()
    result = result.filter(s => s.title.toLowerCase().includes(term))
  }
  result = result.filter(s => baseFilter(s) && matchesTab(s))
  if (orderBy.value === 'lastChange') {
    result = [...result].sort((a, b) => new Date(b.lastChangeUTC || 0).getTime() - new Date(a.lastChangeUTC || 0).getTime())
  } else {
    result = [...result].sort((a, b) => a.title.localeCompare(b.title))
  }
  return result
})

const counts = computed(() => {
  const base = deduplicatedLibrary.value.filter(baseFilter)
  return {
    all: base.length,
    active: base.filter(s => s.status !== SeriesStatus.COMPLETED && s.status !== SeriesStatus.PUBLISHING_FINISHED && s.isActive && !s.pausedDownloads).length,
    paused: base.filter(s => s.pausedDownloads).length,
    unassigned: base.filter(s => s.hasUnknown === true).length,
    completed: base.filter(s => s.status === SeriesStatus.COMPLETED || s.status === SeriesStatus.PUBLISHING_FINISHED).length,
  }
})

const tabOptions = computed(() => [
  { label: `All${counts.value.all ? ` (${counts.value.all})` : ''}`, value: 'all' },
  { label: `Active${counts.value.active ? ` (${counts.value.active})` : ''}`, value: 'active' },
  { label: `Paused${counts.value.paused ? ` (${counts.value.paused})` : ''}`, value: 'paused' },
  { label: `Unassigned${counts.value.unassigned ? ` (${counts.value.unassigned})` : ''}`, value: 'unassigned' },
  { label: `Completed${counts.value.completed ? ` (${counts.value.completed})` : ''}`, value: 'completed' },
])

const genreOptions = computed(() => [
  { label: 'All Genres', value: '__ALL__' },
  ...genres.value.map(g => ({ label: g, value: g })),
])

const providerOptions = computed(() => [
  { label: 'All Sources', value: '__ALL__' },
  ...providers.value.map(p => ({ label: p, value: p })),
])

const orderOptions = [
  { label: 'Alphabetical', value: 'title' },
  { label: 'Last Change', value: 'lastChange' },
]

const router = useRouter()
const toast = useToast()
const verifyAllMutation = useVerifyAll()

function handleSeriesClick(id: string) {
  router.push(`/library/series?id=${id}`)
}

async function handleVerifyAll() {
  await verifyAllMutation.mutateAsync()
  toast.add({ title: 'Library verification queued', color: 'info' })
}
</script>

<template>
  <div>
    <div class="flex items-center flex-wrap gap-2">
      <USelectMenu
        v-model="tab"
        :items="tabOptions"
        value-key="value"
        class="w-40"
      />
      <USelectMenu
        :model-value="selectedGenre ?? '__ALL__'"
        :items="genreOptions"
        value-key="value"
        class="w-40"
        @update:model-value="selectedGenre = $event === '__ALL__' ? null : $event"
      />
      <USelectMenu
        :model-value="selectedProvider ?? '__ALL__'"
        :items="providerOptions"
        value-key="value"
        class="w-48"
        @update:model-value="selectedProvider = $event === '__ALL__' ? null : $event"
      />
      <div class="ml-auto flex items-center gap-2">
        <USelectMenu v-model="orderBy" :items="orderOptions" value-key="value" class="w-32" />
        <USelectMenu v-model="cardWidth" :items="cardWidthOptions" value-key="value" class="w-16" />
        <UButton
          icon="i-lucide-shield-check"
          :label="verifyAllMutation.isPending.value ? 'Verifying...' : 'Verify All'"
          size="sm"
          :loading="verifyAllMutation.isPending.value"
          @click="handleVerifyAll"
        />
        <SeriesAddSeries />
      </div>
    </div>

    <div class="flex flex-wrap gap-4 pt-4">
      <div v-if="filteredLibrary.length === 0" class="text-muted">
        {{ debouncedSearchTerm.trim() ? `No series found matching "${debouncedSearchTerm}"` : 'No series found' }}
      </div>
      <SeriesCard
        v-for="series in filteredLibrary"
        :key="series.id"
        :series="series"
        :card-width="cardWidth"
        :order-by="orderBy"
        @click="handleSeriesClick(series.id)"
      />
    </div>
  </div>
</template>
