<script setup lang="ts">
import type { LinkedSeries } from '~/types'
import { getApiConfig } from '~/utils/api-config'

const keyword = ref('')
const debouncedKeyword = useDebounce(keyword, 500)
const selectedIds = ref<string[]>([])
const selectedSourceIds = ref<string[]>([])

const { data: sources } = useSearchSources()

const sortedSources = computed(() => {
  if (!sources.value) return []
  return [...sources.value].sort((a, b) => a.sourceName.localeCompare(b.sourceName))
})

const sourceItems = computed(() =>
  sortedSources.value.map(s => ({
    label: `${s.sourceName} (${s.language.toUpperCase()})`,
    value: s.sourceId,
  }))
)

const searchParams = computed(() => ({
  keyword: debouncedKeyword.value,
  ...(selectedSourceIds.value.length > 0 && { searchSources: selectedSourceIds.value }),
}))

const { data: results, isLoading } = useSearchSeries(searchParams, computed(() => debouncedKeyword.value.length >= 3))

// Reset selections when search results change
watch(results, () => {
  selectedIds.value = []
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

function toggleSelection(id: string) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) {
    selectedIds.value.splice(idx, 1)
  } else {
    selectedIds.value.push(id)
    // Auto-select linked series
    const series = results.value?.find((s: LinkedSeries) => s.id === id)
    if (series && selectedIds.value.length === 1) {
      series.linkedIds?.forEach((linkedId: string) => {
        if (!selectedIds.value.includes(linkedId)) {
          selectedIds.value.push(linkedId)
        }
      })
    }
  }
}

function getSelectedSeries(): LinkedSeries[] {
  if (!results.value) return []
  return results.value.filter((s: LinkedSeries) => selectedIds.value.includes(s.id))
}

function removeSource(sourceId: string) {
  selectedSourceIds.value = selectedSourceIds.value.filter(id => id !== sourceId)
}

function getSourceLabel(sourceId: string): string {
  const src = sources.value?.find(s => s.sourceId === sourceId)
  return src ? `${src.sourceName} (${src.language.toUpperCase()})` : sourceId
}

const hasSelection = computed(() => selectedIds.value.length > 0)

defineExpose({ getSelectedSeries, hasSelection })
</script>

<template>
  <div class="space-y-4">
    <div class="flex gap-2">
      <UInput
        v-model="keyword"
        type="search"
        placeholder="Search for a series..."
        icon="i-lucide-search"
        class="flex-1"
      />
      <USelectMenu
        v-model="selectedSourceIds"
        :items="sourceItems"
        multiple
        value-key="value"
        placeholder="All sources"
        class="w-52"
        :search-input="false"
      />
    </div>

    <!-- Selected source chips -->
    <div v-if="selectedSourceIds.length > 0" class="flex flex-wrap items-center gap-2">
      <UBadge
        v-for="srcId in selectedSourceIds"
        :key="srcId"
        variant="subtle"
        class="cursor-pointer"
        @click="removeSource(srcId)"
      >
        {{ getSourceLabel(srcId) }}
        <UIcon name="i-lucide-x" class="size-3 ml-1" />
      </UBadge>
    </div>

    <div v-if="isLoading" class="text-center text-muted py-12">
      <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin mx-auto mb-3 opacity-50" />
      <p>Searching...</p>
    </div>

    <div v-else-if="results && results.length > 0">
      <div class="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-5 gap-3 pb-2">
        <div
          v-for="series in results"
          :key="series.id"
          class="cursor-pointer transition-all duration-200 hover:shadow-lg rounded-md overflow-hidden"
          :class="selectedIds.includes(series.id) ? 'ring-2 ring-primary shadow-md' : 'hover:ring-1 hover:ring-gray-300'"
          @click="toggleSelection(series.id)"
        >
          <div class="aspect-[3/4] relative">
            <img
              :src="formatThumbnailUrl(series.thumbnailUrl)"
              :alt="series.title"
              class="object-cover w-full h-full"
              loading="lazy"
              @error="onImgError"
            />
            <UBadge color="neutral" variant="solid" class="absolute top-1 left-1 bg-black/70 text-white text-xs max-w-[94%] truncate">
              {{ series.provider }}
            </UBadge>
            <div v-if="selectedIds.includes(series.id)" class="absolute top-1 right-1 bg-primary text-white rounded-full p-0.5">
              <UIcon name="i-lucide-check" class="size-3.5" />
            </div>
          </div>
          <div
            class="p-2 text-center bg-default"
          >
            <h3 class="text-sm font-medium line-clamp-2">{{ series.title }}</h3>
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="debouncedKeyword.length >= 3" class="text-center text-muted py-12">
      <UIcon name="i-lucide-search-x" class="size-10 mx-auto mb-3 opacity-50" />
      <p>No results found for "{{ debouncedKeyword }}"</p>
      <p class="text-xs mt-1">Try a different search term or change source filter</p>
    </div>

    <div v-else class="text-center text-muted py-12">
      <UIcon name="i-lucide-book-open" class="size-10 mx-auto mb-3 opacity-50" />
      <p>Search for a manga or comic series</p>
      <p class="text-xs mt-1">Type at least 3 characters to start searching</p>
    </div>
  </div>
</template>
