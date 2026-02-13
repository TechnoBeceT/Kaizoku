<script setup lang="ts">
import { type SeriesInfo, SeriesStatus } from '~/types'
import { getStatusDisplay } from '~/utils/series-status'
import { getCountryCodeForLanguage } from '~/utils/language-country-map'

const props = defineProps<{
  series: SeriesInfo
  cardWidth: string
  orderBy: string
}>()

const emit = defineEmits<{
  click: []
}>()

const LAST_CHANGE_COLORS = [
  '00FF00', '22FF00', '44FF00', '66FF00', '88FF00', 'AAFF00', 'CCFF00', 'FFFF00',
  'FFCC00', 'FFAA00', 'FF8800', 'FF6600', 'FF4400', 'FF2200', 'FF0000', 'FF0022',
  'FF0044', 'FF0066', 'FF0088', 'FF00AA', 'FF00CC', 'FF00FF', 'CC00FF', 'AA00FF',
  '8800FF', '6600FF', '4400FF', '2200FF', '0000FF', '2200FF', '4400FF',
]

const ringColor = computed(() => {
  if (!props.series.lastChangeUTC) return null
  const diffDays = Math.floor((Date.now() - new Date(props.series.lastChangeUTC).getTime()) / 86400000)
  if (diffDays < 0 || diffDays > 31) return null
  return LAST_CHANGE_COLORS[Math.min(diffDays, 30)] || null
})

const showRing = computed(() => props.orderBy === 'lastChange' && ringColor.value)

const borderStyle = computed(() => {
  if (!showRing.value) return {}
  return {
    border: `1.5px solid #${ringColor.value}`,
    borderRadius: '6px',
  }
})

const thumbnailUrl = computed(() => {
  return props.series.thumbnailUrl || '/kaizoku.net.png'
})

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}
</script>

<template>
  <div
    :class="['relative rounded-md shadow group transition-all duration-200 cursor-pointer hover:scale-105', cardWidth]"
    :style="{ aspectRatio: '4/6', ...borderStyle }"
    @click="emit('click')"
  >
    <div class="relative w-full h-full rounded-md overflow-hidden">
      <img
        :src="thumbnailUrl"
        :alt="series.title"
        class="rounded-md object-cover w-full h-full"
        loading="lazy"
        @error="onImgError"
      />
      <!-- Provider Badge -->
      <div class="absolute top-1 left-1">
        <UBadge color="neutral" variant="solid" class="bg-black/70 text-white text-xs">
          {{ series.lastChangeProvider?.provider || 'Unknown' }}
        </UBadge>
      </div>
      <!-- Title Bar -->
      <div class="absolute bottom-0 left-0 w-full bg-black/60 text-white font-semibold px-2 py-1 rounded-b-md text-sm text-center truncate">
        {{ series.title }}
      </div>
    </div>
    <!-- Last Chapter Badge -->
    <div v-if="series.lastChapter !== undefined" class="absolute -top-1 -right-1">
      <UBadge
        :color="series.isActive && series.status !== SeriesStatus.DISABLED ? 'primary' : 'neutral'"
        size="xs"
      >
        {{ series.lastChapter }}
      </UBadge>
    </div>
  </div>
</template>
