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

function toggleSelection(index: number) {
  seriesList.value[index].isSelected = !seriesList.value[index].isSelected
  // If the deselected source had useCover or useTitle, reassign to first selected
  const s = seriesList.value[index]
  if (!s.isSelected) {
    const firstSelected = seriesList.value.find(x => x.isSelected)
    if (s.useCover && firstSelected) {
      s.useCover = false
      firstSelected.useCover = true
    }
    if (s.useTitle && firstSelected) {
      s.useTitle = false
      firstSelected.useTitle = true
    }
  }
  emitUpdate()
}

function setCoverSource(id: string) {
  seriesList.value.forEach((s) => {
    s.useCover = s.id === id && s.scanlator === seriesList.value.find(x => x.id === id)?.scanlator
  })
  // Find the exact item since multiple can share same id (different scanlators)
  const idx = seriesList.value.findIndex(s => s.id === id)
  if (idx >= 0) {
    seriesList.value.forEach((s, i) => {
      s.useCover = i === idx
    })
  }
  emitUpdate()
}

function setTitleSource(idx: number) {
  seriesList.value.forEach((s, i) => {
    s.useTitle = i === idx
  })
  emitUpdate()
}

function onDragEnd() {
  // Update importance based on new order
  seriesList.value.forEach((s, i) => {
    s.importance = i
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

    <!-- Source Importance List -->
    <div class="space-y-2">
      <div class="flex items-center gap-2">
        <label class="text-sm font-medium">Source Priority (drag to reorder)</label>
        <UBadge variant="subtle" size="xs">{{ selectedCount }}/{{ seriesList.length }} selected</UBadge>
      </div>
      <draggable
        v-model="seriesList"
        item-key="id"
        handle=".drag-handle"
        ghost-class="opacity-30"
        @end="onDragEnd"
      >
        <template #item="{ element, index }">
          <div
            class="flex items-center gap-3 p-3 mb-2 rounded-lg border transition-colors"
            :class="element.isSelected
              ? 'border-primary/50 bg-default hover:border-primary/70'
              : 'border-default bg-default opacity-50'"
          >
            <!-- Selection Checkbox -->
            <label class="shrink-0 cursor-pointer" @click.stop>
              <input
                type="checkbox"
                :checked="element.isSelected"
                class="accent-primary size-4"
                @change="toggleSelection(index)"
              />
            </label>

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
            <div v-if="element.isSelected" class="flex flex-col gap-2 shrink-0">
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
                  @change="setTitleSource(index)"
                />
                Title
              </label>
            </div>

            <!-- Existing Badge -->
            <UBadge v-if="element.existingProvider" color="warning" size="xs" class="shrink-0">
              EXISTS
            </UBadge>
          </div>
        </template>
      </draggable>
    </div>
  </div>
</template>
