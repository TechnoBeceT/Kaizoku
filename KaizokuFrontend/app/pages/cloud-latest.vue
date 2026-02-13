<script setup lang="ts">
import type { LatestSeriesInfo } from '~/types'

definePageMeta({ layout: 'default' })

const { debouncedSearchTerm } = useSearchState()
const { data: sources } = useSearchSources()

const cardWidthOptions = [
  { value: 'w-20', label: 'XS', text: 'text-[0.4rem]' },
  { value: 'w-32', label: 'S', text: 'text-xs' },
  { value: 'w-45', label: 'M', text: 'text-sm' },
  { value: 'w-58', label: 'L', text: 'text-base' },
  { value: 'w-70', label: 'XL', text: 'text-lg' },
]

const selectedSourceIds = useSessionStorage<string[]>('cloud_sourceIds', [])
const cardWidth = useSessionStorage<string>('cloud_cardWidth', 'w-45')
const browseMode = useSessionStorage<string>('cloud_browseMode', 'latest')

// Source picker (single-select to add sources)
const sourcePicker = ref<string | null>(null)

const items = ref<LatestSeriesInfo[]>([])
const currentPage = ref(0)
const hasMore = ref(true)
const isLoadingMore = ref(false)
const ITEMS_PER_PAGE = 40

// Join selected source IDs for the API call (comma-separated)
const sourceIdParam = computed(() => {
  if (!selectedSourceIds.value || selectedSourceIds.value.length === 0) return undefined
  return selectedSourceIds.value.join(',')
})

const { data: latestData, isLoading } = useLatest(
  computed(() => currentPage.value * ITEMS_PER_PAGE),
  computed(() => ITEMS_PER_PAGE),
  sourceIdParam,
  computed(() => debouncedSearchTerm.value?.trim() || undefined),
  computed(() => true),
  browseMode,
)

// Reset on filter change
watch([() => debouncedSearchTerm.value, () => selectedSourceIds.value, () => browseMode.value], () => {
  items.value = []
  currentPage.value = 0
  hasMore.value = true
})

// Append new data
watch(latestData, (data) => {
  if (!data) return
  if (currentPage.value === 0) {
    items.value = data
  } else {
    items.value = [...items.value, ...data]
  }
  hasMore.value = data.length >= ITEMS_PER_PAGE
  isLoadingMore.value = false
})

function loadMore() {
  if (!hasMore.value || isLoading.value || isLoadingMore.value) return
  isLoadingMore.value = true
  currentPage.value++
}

const sortedSources = computed(() => {
  if (!sources.value) return []
  return [...sources.value].sort((a, b) => a.sourceName.localeCompare(b.sourceName))
})

// Sources available to pick (exclude already selected)
const availableSourceItems = computed(() => [
  { label: 'Add source...', value: '__PICK__' },
  ...sortedSources.value
    .filter(s => !selectedSourceIds.value.includes(s.sourceId))
    .map(s => ({ label: `${s.sourceName} (${s.language.toUpperCase()})`, value: s.sourceId })),
])

const cardSizeItems = computed(() =>
  cardWidthOptions.map(opt => ({ label: opt.label, value: opt.value }))
)

function onSourcePicked(val: string) {
  if (val && val !== '__PICK__') {
    if (!selectedSourceIds.value.includes(val)) {
      selectedSourceIds.value = [...selectedSourceIds.value, val]
    }
  }
  // Reset picker back to placeholder
  nextTick(() => { sourcePicker.value = null })
}

function removeSource(sourceId: string) {
  selectedSourceIds.value = selectedSourceIds.value.filter(id => id !== sourceId)
}

function clearSources() {
  selectedSourceIds.value = []
}

function getSourceLabel(sourceId: string): string {
  const src = sources.value?.find(s => s.sourceId === sourceId)
  return src ? `${src.sourceName} (${src.language.toUpperCase()})` : sourceId
}
</script>

<template>
  <div>
    <!-- Controls Row -->
    <div class="flex items-center gap-3 mb-3">
      <!-- Source Picker -->
      <USelectMenu
        :model-value="sourcePicker ?? '__PICK__'"
        :items="availableSourceItems"
        value-key="value"
        label-key="label"
        class="w-56"
        @update:model-value="onSourcePicked($event)"
      />

      <!-- Latest / Popular Toggle -->
      <div class="flex rounded-md overflow-hidden border border-default">
        <button
          :class="[
            'px-3 py-1.5 text-sm font-medium transition-colors',
            browseMode === 'latest'
              ? 'bg-primary text-white'
              : 'bg-default hover:bg-elevated'
          ]"
          @click="browseMode = 'latest'"
        >
          Latest
        </button>
        <button
          :class="[
            'px-3 py-1.5 text-sm font-medium transition-colors',
            browseMode === 'popular'
              ? 'bg-primary text-white'
              : 'bg-default hover:bg-elevated'
          ]"
          @click="browseMode = 'popular'"
        >
          Popular
        </button>
      </div>

      <!-- Card Size (far right) -->
      <div class="ml-auto w-16">
        <USelectMenu
          v-model="cardWidth"
          :items="cardSizeItems"
          value-key="value"
          label-key="label"
        />
      </div>
    </div>

    <!-- Selected Source Chips -->
    <div v-if="selectedSourceIds.length > 0" class="flex flex-wrap items-center gap-2 mb-3">
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
      <button class="text-xs text-muted hover:text-default transition-colors" @click="clearSources">
        Clear all
      </button>
    </div>

    <!-- Info banner for popular mode without source selection -->
    <div v-if="browseMode === 'popular' && selectedSourceIds.length === 0" class="mb-4 p-3 rounded-md bg-elevated text-sm text-muted">
      <UIcon name="i-lucide-info" class="size-4 mr-1 align-middle" />
      Select one or more sources to browse popular series.
    </div>

    <SeriesCloudLatestGrid
      :items="items"
      :is-loading="isLoading && currentPage === 0"
      :is-loading-more="isLoadingMore"
      :has-more="hasMore"
      :card-width="cardWidth"
      :card-width-options="cardWidthOptions"
      @load-more="loadMore"
    />
  </div>
</template>
