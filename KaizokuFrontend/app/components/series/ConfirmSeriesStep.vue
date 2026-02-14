<script setup lang="ts">
import draggable from 'vuedraggable'
import type { AugmentedResponse, FullSeries } from '~/types'
import { getApiConfig } from '~/utils/api-config'

const props = defineProps<{
  augmented: AugmentedResponse
}>()

const emit = defineEmits<{
  'update:augmented': [value: AugmentedResponse]
}>()

const selectedCategory = ref(props.augmented.categories?.[0] || '')

const seriesList = ref<FullSeries[]>([...props.augmented.series])

// Capture the ORIGINAL base path once — never changes, prevents path duplication
const basePath = props.augmented.storageFolderPath || ''

// Set initial importance and selection state
seriesList.value.forEach((s, i) => {
  s.importance = i
  if (s.isSelected === undefined || s.isSelected === null) {
    s.isSelected = true
  }
})

// Ensure at least one has useCover and useTitle
if (!seriesList.value.some(s => s.useCover)) {
  seriesList.value[0].useCover = true
}
if (!seriesList.value.some(s => s.useTitle)) {
  seriesList.value[0].useTitle = true
}

// Two-zone computed lists
const selectedSeries = computed(() =>
  seriesList.value
    .map((s, i) => ({ series: s, backingIndex: i }))
    .filter(({ series }) => series.isSelected)
    .sort((a, b) => a.series.importance - b.series.importance),
)

const unselectedSeries = computed(() =>
  seriesList.value
    .map((s, i) => ({ series: s, backingIndex: i }))
    .filter(({ series }) => !series.isSelected),
)

// Writable model for vuedraggable — maps selected items in order
const selectedModel = computed({
  get() {
    return selectedSeries.value.map(({ series }) => series)
  },
  set(newOrder: FullSeries[]) {
    // Recalculate importance based on new drag order
    newOrder.forEach((s, i) => {
      s.importance = i
    })
    emitUpdate()
  },
})

const selectedCount = computed(() => seriesList.value.filter(s => s.isSelected).length)

const titleSource = computed(() => seriesList.value.find(s => s.useTitle && s.isSelected))

const storagePath = computed(() => {
  const title = titleSource.value?.suggestedFilename || titleSource.value?.title || 'Unknown'
  if (props.augmented.useCategoriesForPath && selectedCategory.value) {
    return `${basePath}/${selectedCategory.value}/${title}`
  }
  return `${basePath}/${title}`
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

function selectSource(backingIndex: number) {
  const s = seriesList.value[backingIndex]
  s.isSelected = true
  // Assign importance to end of selected list
  const maxImportance = Math.max(-1, ...seriesList.value.filter(x => x.isSelected && x !== s).map(x => x.importance))
  s.importance = maxImportance + 1
  // If first selection, auto-assign cover and title
  if (selectedCount.value === 1) {
    s.useCover = true
    s.useTitle = true
  }
  emitUpdate()
}

function deselectSource(backingIndex: number) {
  const s = seriesList.value[backingIndex]
  s.isSelected = false
  // Reassign cover/title if this source had them
  const firstSelected = seriesList.value.find(x => x.isSelected && x !== s)
  if (s.useCover && firstSelected) {
    s.useCover = false
    firstSelected.useCover = true
  }
  if (s.useTitle && firstSelected) {
    s.useTitle = false
    firstSelected.useTitle = true
  }
  // Renumber selected items sequentially
  renumberSelected()
  emitUpdate()
}

function renumberSelected() {
  const selected = seriesList.value
    .filter(s => s.isSelected)
    .sort((a, b) => a.importance - b.importance)
  selected.forEach((s, i) => {
    s.importance = i
  })
}

function setCoverSource(id: string) {
  const idx = seriesList.value.findIndex(s => s.id === id && s.isSelected)
  if (idx >= 0) {
    seriesList.value.forEach((s, i) => {
      s.useCover = i === idx
    })
  }
  emitUpdate()
}

function setTitleSource(backingIndex: number) {
  seriesList.value.forEach((s, i) => {
    s.useTitle = i === backingIndex
  })
  emitUpdate()
}

function emitUpdate() {
  const updated: AugmentedResponse = {
    ...props.augmented,
    series: seriesList.value.filter(s => s.isSelected),
    storageFolderPath: storagePath.value,
  }
  emit('update:augmented', updated)
}

// Emit initial state
emitUpdate()
</script>

<template>
  <div class="space-y-5">
    <!-- Storage Path -->
    <div v-if="augmented.useCategoriesForPath" class="space-y-2">
      <label class="text-sm font-medium">Category</label>
      <USelectMenu
        v-model="selectedCategory"
        :items="augmented.categories"
        class="w-full max-w-xs"
        @update:model-value="emitUpdate()"
      />
      <p class="text-xs text-muted truncate">
        Path: {{ storagePath }}
      </p>
    </div>

    <!-- Selected Sources (draggable, numbered) -->
    <div class="space-y-2">
      <div class="flex items-center gap-2">
        <label class="text-sm font-medium">Selected Sources (drag to reorder)</label>
        <UBadge variant="subtle" size="xs">{{ selectedCount }}/{{ seriesList.length }}</UBadge>
      </div>

      <div v-if="selectedCount === 0" class="text-center text-muted py-6 border border-dashed rounded-lg">
        No sources selected. Add sources from the list below.
      </div>

      <draggable
        v-else
        v-model="selectedModel"
        item-key="id"
        handle=".drag-handle"
        ghost-class="opacity-30"
      >
        <template #item="{ element, index }">
          <div class="flex items-center gap-3 p-3 mb-2 rounded-lg border border-primary/50 bg-default hover:border-primary/70 transition-colors">
            <!-- Drag Handle -->
            <div class="drag-handle cursor-grab active:cursor-grabbing text-muted hover:text-default">
              <UIcon name="i-lucide-grip-vertical" class="size-5" />
            </div>

            <!-- Importance Badge -->
            <UBadge :color="index === 0 ? 'primary' : 'neutral'" variant="solid" class="shrink-0 w-8 justify-center">
              #{{ index + 1 }}
            </UBadge>

            <!-- Thumbnail -->
            <img
              :src="formatThumbnailUrl(element.thumbnailUrl)"
              :alt="element.title"
              class="w-16 h-22 rounded object-cover shrink-0"
              @error="onImgError"
            />

            <!-- Provider Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-medium text-sm truncate">{{ element.provider }}</span>
                <span v-if="element.scanlator && element.scanlator !== element.provider" class="text-xs text-muted truncate">
                  {{ element.scanlator }}
                </span>
                <span class="text-xs px-1 py-0.5 rounded bg-muted shrink-0">{{ element.lang?.toUpperCase() }}</span>
              </div>
              <div class="text-xs text-muted mt-0.5">
                {{ element.chapterCount }} chapters
                <span v-if="element.chapterList"> &middot; {{ element.chapterList }}</span>
              </div>
              <div v-if="element.artist || element.author" class="text-xs text-muted mt-0.5">
                <span v-if="element.author">{{ element.author }}</span>
                <span v-if="element.artist && element.artist !== element.author"> / {{ element.artist }}</span>
              </div>
            </div>

            <!-- Cover / Title Radio -->
            <div class="flex flex-col gap-2 shrink-0">
              <label class="flex items-center gap-1.5 text-xs cursor-pointer" @click.stop>
                <input
                  type="radio"
                  name="coverSource"
                  :checked="element.useCover"
                  class="accent-primary"
                  @change="setCoverSource(element.id)"
                />
                Cover
              </label>
              <label class="flex items-center gap-1.5 text-xs cursor-pointer" @click.stop>
                <input
                  type="radio"
                  name="titleSource"
                  :checked="element.useTitle"
                  class="accent-primary"
                  @change="setTitleSource(seriesList.indexOf(element))"
                />
                Title
              </label>
            </div>

            <!-- Remove Button -->
            <button
              class="shrink-0 p-1 rounded hover:bg-error/10 text-muted hover:text-error transition-colors"
              title="Remove from selection"
              @click="deselectSource(seriesList.indexOf(element))"
            >
              <UIcon name="i-lucide-x" class="size-4" />
            </button>

            <!-- Existing Badge -->
            <UBadge v-if="element.existingProvider" color="warning" size="xs" class="shrink-0">
              EXISTS
            </UBadge>
          </div>
        </template>
      </draggable>
    </div>

    <!-- Available Sources (not selected) -->
    <div v-if="unselectedSeries.length > 0" class="space-y-2">
      <label class="text-sm font-medium text-muted">Available Sources</label>

      <div
        v-for="{ series: element, backingIndex } in unselectedSeries"
        :key="`avail-${element.id}-${backingIndex}`"
        class="flex items-center gap-3 p-3 mb-2 rounded-lg border border-default bg-default opacity-60 hover:opacity-80 transition-opacity"
      >
        <!-- Thumbnail -->
        <img
          :src="formatThumbnailUrl(element.thumbnailUrl)"
          :alt="element.title"
          class="w-12 h-16 rounded object-cover shrink-0"
          @error="onImgError"
        />

        <!-- Provider Info -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="font-medium text-sm truncate">{{ element.provider }}</span>
            <span v-if="element.scanlator && element.scanlator !== element.provider" class="text-xs text-muted truncate">
              {{ element.scanlator }}
            </span>
            <span class="text-xs px-1 py-0.5 rounded bg-muted shrink-0">{{ element.lang?.toUpperCase() }}</span>
          </div>
          <div class="text-xs text-muted mt-0.5">
            {{ element.chapterCount }} chapters
            <span v-if="element.chapterList"> &middot; {{ element.chapterList }}</span>
          </div>
        </div>

        <!-- Add Button -->
        <button
          class="shrink-0 p-1.5 rounded hover:bg-primary/10 text-muted hover:text-primary transition-colors"
          title="Add to selection"
          @click="selectSource(backingIndex)"
        >
          <UIcon name="i-lucide-plus" class="size-5" />
        </button>

        <!-- Existing Badge -->
        <UBadge v-if="element.existingProvider" color="warning" size="xs" class="shrink-0">
          EXISTS
        </UBadge>
      </div>
    </div>
  </div>
</template>
