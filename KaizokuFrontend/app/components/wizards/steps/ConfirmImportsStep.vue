<script setup lang="ts">
import { ImportStatus, Action, type ImportInfo, type SmallSeries, type LinkedSeries } from '~/types'
import { getApiConfig } from '~/utils/api-config'
import { setupWizardService } from '~/services/setupWizardService'

const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
}>()

// --- Data fetching ---
const { data: apiImports, refetch } = useSetupWizardImports()

// --- Local state (local-first: all changes go here immediately, synced to backend with debounce) ---
const localImports = ref<ImportInfo[]>([])
const activeTab = ref('add')

// Sync from API → local on fetch, fix importance and auto-move no-source imports
watch(apiImports, (val) => {
  if (val) {
    const imports: ImportInfo[] = JSON.parse(JSON.stringify(val))
    for (const imp of imports) {
      // Auto-move imports with Import status but no sources to Not Matched
      if (imp.status === ImportStatus.Import && (!imp.series || imp.series.length === 0)) {
        imp.status = ImportStatus.Skip
        imp.action = Action.Skip
        setupWizardService.updateImport(imp).catch(() => {})
      }
      // Fix all-zero importance: assign sequential values if all sources have same importance
      if (imp.series && imp.series.length > 1) {
        const allSame = imp.series.every(s => s.importance === imp.series![0].importance)
        if (allSame) {
          imp.series.forEach((s, i) => { s.importance = i })
        }
      }
      // Default isSelected to true for all sources
      if (imp.series) {
        imp.series.forEach((s) => {
          if (s.isSelected === undefined || s.isSelected === null) {
            s.isSelected = true
          }
        })
      }
    }
    localImports.value = imports
  }
})

// --- Debounced backend sync ---
const debounceTimers: Record<string, ReturnType<typeof setTimeout>> = {}

function scheduleSyncToBackend(importInfo: ImportInfo) {
  const key = importInfo.path
  if (debounceTimers[key]) clearTimeout(debounceTimers[key])
  debounceTimers[key] = setTimeout(async () => {
    try {
      // Only sync selected sources to backend
      const toSync = {
        ...importInfo,
        series: importInfo.series?.filter(s => s.isSelected),
      }
      await setupWizardService.updateImport(toSync)
    } catch {
      props.setError('Failed to sync changes to backend')
    }
    delete debounceTimers[key]
  }, 3000)
}

// Flush all pending debounces on unmount
onUnmounted(() => {
  for (const key of Object.keys(debounceTimers)) {
    clearTimeout(debounceTimers[key])
    const imp = localImports.value.find(i => i.path === key)
    if (imp) {
      const toSync = {
        ...imp,
        series: imp.series?.filter(s => s.isSelected),
      }
      setupWizardService.updateImport(toSync).catch(() => {})
    }
    delete debounceTimers[key]
  }
})

// --- Helpers ---
function findImport(path: string): ImportInfo | undefined {
  return localImports.value.find(i => i.path === path)
}

function updateLocalImport(path: string, updates: Partial<ImportInfo>) {
  const idx = localImports.value.findIndex(i => i.path === path)
  if (idx === -1) return
  const updated: ImportInfo = { ...localImports.value[idx]!, ...updates } as ImportInfo
  localImports.value[idx] = updated
  scheduleSyncToBackend(updated)
}

function formatThumbnailUrl(url?: string | null): string {
  if (!url) return '/kaizoku.net.png'
  if (url.startsWith('http')) return url
  const config = getApiConfig()
  return config.baseUrl ? `${config.baseUrl}/api/${url}` : `/api/${url}`
}

function getThumbnail(item: ImportInfo): string {
  const preferred = item.series?.find(s => s.preferred)
  const thumb = preferred?.thumbnailUrl ?? item.series?.[0]?.thumbnailUrl
  return formatThumbnailUrl(thumb)
}

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}

// Split series into selected (sorted by importance) and unselected
function selectedSeries(series: SmallSeries[]): { series: SmallSeries; origIdx: number }[] {
  return series
    .map((s, i) => ({ series: s, origIdx: i }))
    .filter(({ series }) => series.isSelected)
    .sort((a, b) => {
      if (a.series.preferred !== b.series.preferred) return a.series.preferred ? -1 : 1
      return a.series.importance - b.series.importance
    })
}

function unselectedSeries(series: SmallSeries[]): { series: SmallSeries; origIdx: number }[] {
  return series
    .map((s, i) => ({ series: s, origIdx: i }))
    .filter(({ series }) => !series.isSelected)
}

function selectedCount(series: SmallSeries[]): number {
  return series.filter(s => s.isSelected).length
}

// --- Tab filtering ---
// "Add" tab: only imports with sources (no-source imports belong in Not Matched)
const filteredImports = computed(() => {
  switch (activeTab.value) {
    case 'add': return localImports.value.filter(i => i.status === ImportStatus.Import && i.series?.length)
    case 'finished': return localImports.value.filter(i => i.status === ImportStatus.Completed)
    case 'already': return localImports.value.filter(i => i.status === ImportStatus.DoNotChange)
    case 'notmatched': return localImports.value.filter(i => i.status === ImportStatus.Skip)
    default: return localImports.value
  }
})

const counts = computed(() => ({
  add: localImports.value.filter(i => i.status === ImportStatus.Import && i.series?.length).length,
  finished: localImports.value.filter(i => i.status === ImportStatus.Completed).length,
  already: localImports.value.filter(i => i.status === ImportStatus.DoNotChange).length,
  notmatched: localImports.value.filter(i => i.status === ImportStatus.Skip).length,
}))

const tabs = computed(() => [
  { label: `Add (${counts.value.add})`, value: 'add' },
  { label: `Finished (${counts.value.finished})`, value: 'finished' },
  { label: `Already Imported (${counts.value.already})`, value: 'already' },
  { label: `Not Matched (${counts.value.notmatched})`, value: 'notmatched' },
])

// --- Import card actions ---
function handleMismatch(path: string) {
  updateLocalImport(path, { status: ImportStatus.Skip, action: Action.Skip })
}

function handleInclude(path: string) {
  const imp = findImport(path)
  // Only allow Include if the import has sources
  if (!imp?.series?.length) return
  updateLocalImport(path, { status: ImportStatus.Import, action: Action.Add })
}

function handleChapterChange(path: string, value: string) {
  const chapter = parseFloat(value) || 0
  updateLocalImport(path, { continueAfterChapter: chapter })
}

// --- Source selection ---
function toggleSourceSelection(importPath: string, seriesIndex: number) {
  const imp = findImport(importPath)
  if (!imp?.series) return

  const series = [...imp.series]
  const current = series[seriesIndex]
  if (!current) return

  const wasSelected = current.isSelected
  series[seriesIndex] = { ...current, isSelected: !wasSelected }

  if (wasSelected) {
    // Deselecting: reassign cover/title if needed
    const remaining = series.filter((s, i) => s.isSelected && i !== seriesIndex)
    if (current.useCover && remaining.length > 0) {
      series[seriesIndex] = { ...series[seriesIndex], useCover: false }
      const firstIdx = series.findIndex((s, i) => s.isSelected && i !== seriesIndex)
      if (firstIdx >= 0) series[firstIdx] = { ...series[firstIdx], useCover: true }
    }
    if (current.useTitle && remaining.length > 0) {
      series[seriesIndex] = { ...series[seriesIndex], useTitle: false }
      const firstIdx = series.findIndex((s, i) => s.isSelected && i !== seriesIndex)
      if (firstIdx >= 0) series[firstIdx] = { ...series[firstIdx], useTitle: true }
    }
    // Renumber selected items
    const selected = series.filter(s => s.isSelected).sort((a, b) => a.importance - b.importance)
    selected.forEach((s, i) => { s.importance = i })
  } else {
    // Selecting: add to end
    const maxImportance = Math.max(-1, ...series.filter(s => s.isSelected).map(s => s.importance))
    series[seriesIndex] = { ...series[seriesIndex], importance: maxImportance + 1 }
    // If first selection, auto-assign cover and title
    if (series.filter(s => s.isSelected).length === 1) {
      series[seriesIndex] = { ...series[seriesIndex], useCover: true, useTitle: true }
    }
  }

  const updated = { ...imp, series }
  const idx = localImports.value.findIndex(i => i.path === importPath)
  if (idx !== -1) {
    localImports.value[idx] = updated
    scheduleSyncToBackend(updated)
  }
}

// --- Series card actions ---
function togglePreferred(importPath: string, seriesIndex: number) {
  const imp = findImport(importPath)
  if (!imp?.series) return
  // Only allow toggling preferred on selected sources
  if (!imp.series[seriesIndex]?.isSelected) return
  const updated = {
    ...imp,
    series: imp.series.map((s, idx) => ({
      ...s,
      preferred: idx === seriesIndex ? !s.preferred : s.preferred,
    })),
  }
  const idx = localImports.value.findIndex(i => i.path === importPath)
  if (idx !== -1) {
    localImports.value[idx] = updated
    scheduleSyncToBackend(updated)
  }
}

function toggleSwitch(importPath: string, seriesIndex: number, prop: 'useCover' | 'useTitle', value: boolean) {
  const imp = findImport(importPath)
  if (!imp?.series) return
  const updated = {
    ...imp,
    series: imp.series.map((s, idx) =>
      idx === seriesIndex ? { ...s, [prop]: value } : s,
    ),
  }
  const idx = localImports.value.findIndex(i => i.path === importPath)
  if (idx !== -1) {
    localImports.value[idx] = updated
    scheduleSyncToBackend(updated)
  }
}

function movePriority(importPath: string, seriesIndex: number, direction: 'up' | 'down') {
  const imp = findImport(importPath)
  if (!imp?.series) return
  const series = [...imp.series]
  const current = series[seriesIndex]
  if (!current) return

  // Only move among selected sources
  const selectedSorted = series
    .map((s, i) => ({ s, i }))
    .filter(item => item.s.isSelected)
    .sort((a, b) => a.s.importance - b.s.importance)

  const sortedIdx = selectedSorted.findIndex(item => item.i === seriesIndex)
  const swapSortedIdx = direction === 'up' ? sortedIdx - 1 : sortedIdx + 1
  if (swapSortedIdx < 0 || swapSortedIdx >= selectedSorted.length) return

  const swapOrigIdx = selectedSorted[swapSortedIdx].i
  // Swap importance values
  const tempImportance = series[seriesIndex].importance
  series[seriesIndex] = { ...series[seriesIndex], importance: series[swapOrigIdx].importance }
  series[swapOrigIdx] = { ...series[swapOrigIdx], importance: tempImportance }

  const updated = { ...imp, series }
  const idx = localImports.value.findIndex(i => i.path === importPath)
  if (idx !== -1) {
    localImports.value[idx] = updated
    scheduleSyncToBackend(updated)
  }
}

// --- Search modal ---
const searchModalOpen = ref(false)
const searchTarget = ref<ImportInfo | null>(null)
const searchKeyword = ref('')
const debouncedKeyword = useDebounce(searchKeyword, 500)
const selectedSearchIds = ref<string[]>([])
const isAugmenting = ref(false)

const searchParams = computed(() => ({
  keyword: debouncedKeyword.value,
}))

const { data: searchResults, isLoading: isSearching } = useSearchSeries(
  searchParams,
  computed(() => searchModalOpen.value && debouncedKeyword.value.length >= 3),
)

function openSearch(item: ImportInfo) {
  searchTarget.value = item
  searchKeyword.value = item.title || ''
  selectedSearchIds.value = []
  searchModalOpen.value = true
}

function toggleSearchSelection(id: string) {
  const idx = selectedSearchIds.value.indexOf(id)
  if (idx >= 0) {
    selectedSearchIds.value.splice(idx, 1)
  } else {
    selectedSearchIds.value.push(id)
    // Auto-select linked series on first selection
    const series = searchResults.value?.find((s: LinkedSeries) => s.id === id)
    if (series && selectedSearchIds.value.length === 1) {
      series.linkedIds?.forEach((linkedId: string) => {
        if (!selectedSearchIds.value.includes(linkedId)) {
          selectedSearchIds.value.push(linkedId)
        }
      })
    }
  }
}

async function confirmSearch() {
  if (!searchTarget.value || selectedSearchIds.value.length === 0 || !searchResults.value) return

  const selected = searchResults.value.filter((s: LinkedSeries) => selectedSearchIds.value.includes(s.id))
  isAugmenting.value = true

  try {
    const updatedImport = await setupWizardService.augmentSeries(searchTarget.value.path, selected)
    // Default isSelected for new sources
    if (updatedImport.series) {
      updatedImport.series.forEach((s: SmallSeries) => {
        if (s.isSelected === undefined || s.isSelected === null) {
          s.isSelected = true
        }
      })
    }
    // Replace in local state and set to Import (now has sources, goes to Add tab)
    const idx = localImports.value.findIndex(i => i.path === searchTarget.value!.path)
    if (idx !== -1) {
      updatedImport.status = ImportStatus.Import
      updatedImport.action = Action.Add
      localImports.value[idx] = updatedImport
      scheduleSyncToBackend(updatedImport)
    }
    searchModalOpen.value = false
  } catch {
    props.setError('Failed to attach search results')
  } finally {
    isAugmenting.value = false
  }
}

// --- Lifecycle ---
onMounted(async () => {
  props.setIsLoading(true)
  props.setError(null)
  try {
    await refetch()
  } catch {
    props.setError('Failed to load imports')
  } finally {
    props.setIsLoading(false)
    props.setCanProgress(true)
  }
})
</script>

<template>
  <div class="space-y-4">
    <p class="text-sm text-muted">
      Review the imported series below. Items marked as <strong>Not Matched</strong> will not be imported.
      You can move items between tabs and search for correct matches.
    </p>

    <!-- Tabs -->
    <div class="flex gap-2 flex-wrap">
      <UButton
        v-for="tab in tabs"
        :key="tab.value"
        :variant="activeTab === tab.value ? 'solid' : 'ghost'"
        size="sm"
        @click="activeTab = tab.value"
      >
        {{ tab.label }}
      </UButton>
    </div>

    <!-- Import List -->
    <div class="max-h-[55vh] overflow-y-auto space-y-3 pr-1">
      <div v-if="!filteredImports.length" class="text-center text-muted py-8">
        {{ activeTab === 'notmatched' ? 'No unmatched series' : activeTab === 'add' ? 'No series to import' : 'No items in this category' }}
      </div>

      <!-- Import Card -->
      <UCard v-for="item in filteredImports" :key="item.path" class="w-full">
        <!-- Header: thumbnail + title + controls -->
        <div class="flex gap-3">
          <!-- Thumbnail -->
          <div class="flex-shrink-0">
            <img
              :src="getThumbnail(item)"
              :alt="item.title"
              class="w-24 h-36 object-cover rounded-lg"
              loading="lazy"
              @error="onImgError"
            />
          </div>

          <!-- Content -->
          <div class="flex-1 min-w-0 space-y-2">
            <!-- Title row -->
            <div class="flex items-start justify-between gap-2">
              <div class="flex-1 min-w-0">
                <h4 class="text-sm font-semibold line-clamp-2">{{ item.title }}</h4>
                <p class="text-xs text-muted truncate">{{ item.path }}</p>
              </div>

              <!-- Action buttons -->
              <div class="flex items-center gap-2 flex-shrink-0">
                <!-- Continue After Chapter -->
                <div class="flex items-center gap-1">
                  <span class="text-xs text-muted whitespace-nowrap">After Ch:</span>
                  <UInput
                    type="number"
                    :model-value="item.continueAfterChapter ?? 0"
                    size="xs"
                    class="w-16"
                    @update:model-value="handleChapterChange(item.path, String($event))"
                  />
                </div>

                <!-- Search button (Not Matched tab or no series) -->
                <UButton
                  v-if="activeTab === 'notmatched' || !item.series?.length"
                  icon="i-lucide-search"
                  label="Search"
                  size="xs"
                  @click="openSearch(item)"
                />

                <!-- Mismatch button (Add / Finished tabs) -->
                <UButton
                  v-if="activeTab === 'add' || activeTab === 'finished'"
                  icon="i-lucide-x"
                  label="Mismatch"
                  size="xs"
                  variant="outline"
                  color="warning"
                  @click="handleMismatch(item.path)"
                />

                <!-- Include button (Not Matched tab, only if has sources) -->
                <UButton
                  v-if="activeTab === 'notmatched' && item.series?.length"
                  icon="i-lucide-plus"
                  label="Include"
                  size="xs"
                  color="success"
                  @click="handleInclude(item.path)"
                />
              </div>
            </div>

            <!-- Selected Sources -->
            <template v-if="item.series?.length">
              <div v-if="selectedCount(item.series) > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
                <div
                  v-for="{ series, origIdx } in selectedSeries(item.series)"
                  :key="`sel-${series.id}-${series.provider}-${series.scanlator ?? ''}`"
                  class="flex flex-col gap-1.5 px-2.5 py-2 rounded-lg border cursor-pointer transition-all duration-200 overflow-hidden"
                  :class="series.preferred
                    ? 'bg-primary text-primary-foreground shadow-md border-primary ring-1 ring-primary'
                    : 'bg-muted/50 hover:bg-muted border-border hover:border-primary/60'"
                  @click="togglePreferred(item.path, origIdx)"
                >
                  <!-- Series info -->
                  <div class="flex flex-col text-left min-w-0">
                    <div class="flex items-center gap-1.5">
                      <UBadge :color="series.importance === 0 ? 'primary' : 'neutral'" variant="solid" class="text-[10px] px-1.5 py-0 shrink-0">
                        #{{ series.importance + 1 }}
                      </UBadge>
                      <span class="font-medium text-xs truncate" :title="series.title">
                        {{ series.title }}
                      </span>
                    </div>
                    <span class="text-xs truncate" :class="series.preferred ? 'opacity-90' : 'text-muted-foreground'">
                      <template v-if="series.url">
                        <a
                          :href="series.url"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="inline-flex items-center gap-0.5 hover:underline"
                          @click.stop
                        >
                          {{ series.provider }}
                          <UIcon name="i-lucide-external-link" class="size-3" />
                        </a>
                      </template>
                      <template v-else>{{ series.provider }}</template>
                      <template v-if="series.scanlator && series.scanlator !== series.provider">
                        &middot; {{ series.scanlator }}
                      </template>
                    </span>
                    <span class="text-xs font-medium" :class="series.preferred ? 'opacity-90' : 'text-muted-foreground'">
                      {{ series.chapterCount }} ch
                      <template v-if="series.lastChapter">
                        &middot; Last: {{ series.lastChapter }}
                      </template>
                    </span>
                    <span v-if="series.chapterList" class="text-[10px] truncate" :class="series.preferred ? 'opacity-80' : 'text-muted-foreground/70'" :title="series.chapterList">
                      {{ series.lang }} &middot; {{ series.chapterList }}
                    </span>
                    <span v-else class="text-[10px]" :class="series.preferred ? 'opacity-80' : 'text-muted-foreground/70'">
                      {{ series.lang }}
                    </span>
                  </div>

                  <!-- Controls — priority arrows + switches + remove -->
                  <div
                    class="flex items-center flex-wrap gap-x-3 gap-y-1 pt-1 border-t"
                    :class="series.preferred ? 'border-primary-foreground/30' : 'border-border'"
                    @click.stop
                  >
                    <!-- Priority up/down -->
                    <div class="inline-flex items-center gap-0.5">
                      <button
                        class="p-0.5 rounded hover:bg-black/10 dark:hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed"
                        :disabled="series.importance === 0"
                        title="Higher priority"
                        @click="movePriority(item.path, origIdx, 'up')"
                      >
                        <UIcon name="i-lucide-chevron-up" class="size-3.5" />
                      </button>
                      <button
                        class="p-0.5 rounded hover:bg-black/10 dark:hover:bg-white/20 disabled:opacity-30 disabled:cursor-not-allowed"
                        :disabled="series.importance >= selectedCount(item.series!) - 1"
                        title="Lower priority"
                        @click="movePriority(item.path, origIdx, 'down')"
                      >
                        <UIcon name="i-lucide-chevron-down" class="size-3.5" />
                      </button>
                    </div>
                    <label class="inline-flex items-center gap-1 cursor-pointer">
                      <USwitch
                        :model-value="series.useCover"
                        class="scale-75 origin-left"
                        @update:model-value="toggleSwitch(item.path, origIdx, 'useCover', $event)"
                      />
                      <span class="text-[10px]" :class="series.preferred ? 'text-primary-foreground/90' : 'text-muted-foreground'">Cover</span>
                    </label>
                    <label class="inline-flex items-center gap-1 cursor-pointer">
                      <USwitch
                        :model-value="series.useTitle"
                        class="scale-75 origin-left"
                        @update:model-value="toggleSwitch(item.path, origIdx, 'useTitle', $event)"
                      />
                      <span class="text-[10px]" :class="series.preferred ? 'text-primary-foreground/90' : 'text-muted-foreground'">Title</span>
                    </label>
                    <!-- Remove from selection -->
                    <button
                      class="p-0.5 rounded hover:bg-black/10 dark:hover:bg-white/20 ml-auto"
                      title="Remove from selection"
                      @click="toggleSourceSelection(item.path, origIdx)"
                    >
                      <UIcon name="i-lucide-x" class="size-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Unselected Sources -->
              <div v-if="unselectedSeries(item.series).length > 0">
                <p class="text-[10px] text-muted-foreground mb-1">Available sources</p>
                <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-1.5">
                  <div
                    v-for="{ series, origIdx } in unselectedSeries(item.series)"
                    :key="`avail-${series.id}-${series.provider}-${series.scanlator ?? ''}`"
                    class="flex items-center gap-1.5 px-2 py-1.5 rounded-lg border border-border bg-muted/30 opacity-60 hover:opacity-80 transition-opacity cursor-pointer"
                    title="Click to add to selection"
                    @click="toggleSourceSelection(item.path, origIdx)"
                  >
                    <UIcon name="i-lucide-plus" class="size-3 shrink-0 text-muted-foreground" />
                    <span class="text-[10px] truncate">{{ series.provider }}</span>
                    <span v-if="series.scanlator && series.scanlator !== series.provider" class="text-[10px] text-muted-foreground truncate">
                      {{ series.scanlator }}
                    </span>
                    <span class="text-[10px] text-muted-foreground shrink-0">{{ series.lang }}</span>
                  </div>
                </div>
              </div>
            </template>

            <!-- No series message -->
            <div v-else class="text-xs text-muted py-2">
              No sources found. Use the Search button to find matching sources.
            </div>
          </div>
        </div>
      </UCard>
    </div>

    <!-- Search Modal -->
    <UModal v-model:open="searchModalOpen" :ui="{ content: 'sm:max-w-4xl max-h-[85vh]' }">
      <template #body>
        <div class="space-y-4 p-4">
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">
              Search Sources for "{{ searchTarget?.title }}"
            </h3>
            <UButton icon="i-lucide-x" variant="ghost" size="xs" @click="searchModalOpen = false" />
          </div>

          <UInput
            v-model="searchKeyword"
            type="search"
            placeholder="Search for a series..."
            icon="i-lucide-search"
          />

          <!-- Loading -->
          <div v-if="isSearching" class="text-center text-muted py-8">
            <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin mx-auto mb-3 opacity-50" />
            <p>Searching...</p>
          </div>

          <!-- Results grid -->
          <div v-else-if="searchResults && searchResults.length > 0" class="max-h-[50vh] overflow-y-auto">
            <div class="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-5 gap-3 pb-2">
              <div
                v-for="series in searchResults"
                :key="series.id"
                class="cursor-pointer transition-all duration-200 hover:shadow-lg rounded-md overflow-hidden"
                :class="selectedSearchIds.includes(series.id) ? 'ring-2 ring-primary shadow-md' : 'hover:ring-1 hover:ring-gray-300'"
                @click="toggleSearchSelection(series.id)"
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
                  <div v-if="selectedSearchIds.includes(series.id)" class="absolute top-1 right-1 bg-primary text-white rounded-full p-0.5">
                    <UIcon name="i-lucide-check" class="size-3.5" />
                  </div>
                </div>
                <div class="p-2 text-center bg-default">
                  <h3 class="text-sm font-medium line-clamp-2">{{ series.title }}</h3>
                </div>
              </div>
            </div>
          </div>

          <!-- No results -->
          <div v-else-if="debouncedKeyword.length >= 3" class="text-center text-muted py-8">
            <UIcon name="i-lucide-search-x" class="size-10 mx-auto mb-3 opacity-50" />
            <p>No results found for "{{ debouncedKeyword }}"</p>
          </div>

          <!-- Prompt -->
          <div v-else class="text-center text-muted py-8">
            <UIcon name="i-lucide-book-open" class="size-10 mx-auto mb-3 opacity-50" />
            <p>Type at least 3 characters to search</p>
          </div>

          <!-- Footer -->
          <div class="flex justify-end gap-2 pt-2 border-t">
            <UButton variant="outline" label="Cancel" @click="searchModalOpen = false" />
            <UButton
              label="Confirm Selection"
              :disabled="selectedSearchIds.length === 0"
              :loading="isAugmenting"
              @click="confirmSearch"
            />
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>
