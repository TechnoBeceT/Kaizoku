<script setup lang="ts">
import { type LatestSeriesInfo, InLibraryStatus } from '~/types'
import { getStatusDisplay } from '~/utils/series-status'
import { getCountryCodeForLanguage } from '~/utils/language-country-map'

const props = defineProps<{
  items: LatestSeriesInfo[]
  isLoading: boolean
  isLoadingMore: boolean
  hasMore: boolean
  cardWidth: string
  cardWidthOptions: { value: string; label: string; text: string }[]
}>()

const emit = defineEmits<{
  loadMore: []
}>()

const router = useRouter()

const FETCH_DATE_COLORS = [
  '00FF00', '22FF00', '44FF00', '66FF00', '88FF00', 'AAFF00', 'CCFF00', 'FFFF00',
  'FFCC00', 'FFAA00', 'FF8800', 'FF6600', 'FF4400', 'FF2200', 'FF0000', 'FF0022',
  'FF0044', 'FF0066', 'FF0088', 'FF00AA', 'FF00CC', 'FF00FF', 'CC00FF', 'AA00FF',
  '8800FF', '6600FF', '4400FF', '2200FF', '0000FF', '2200FF', '4400FF',
]

function getFetchDateRingColor(fetchDate?: string): string | null {
  if (!fetchDate) return null
  const now = new Date()
  const nowUTC = new Date(now.getTime() + now.getTimezoneOffset() * 60000)
  const fetchUTC = new Date(fetchDate)
  const nowDay = Math.floor(nowUTC.getTime() / 86400000)
  const fetchDay = Math.floor(fetchUTC.getTime() / 86400000)
  const diff = nowDay - fetchDay
  if (diff < 0 || diff > 31) return null
  return FETCH_DATE_COLORS[Math.min(diff, 30)] || null
}

const textSize = computed(() =>
  props.cardWidthOptions.find(opt => opt.value === props.cardWidth)?.text || 'text-sm'
)

// Intersection observer for infinite scroll
const loadMoreRef = ref<HTMLDivElement>()
let observer: IntersectionObserver | null = null

onMounted(() => {
  observer = new IntersectionObserver(
    (entries) => {
      if (entries[0]?.isIntersecting && props.hasMore && !props.isLoadingMore) {
        emit('loadMore')
      }
    },
    { threshold: 0.1, rootMargin: '200px' }
  )
})

watch(loadMoreRef, (el) => {
  if (el && observer) observer.observe(el)
})

onUnmounted(() => {
  observer?.disconnect()
})

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}

function handleCardClick(item: LatestSeriesInfo) {
  if (item.seriesId) {
    router.push(`/library/series?id=${item.seriesId}`)
  } else if (item.url) {
    window.open(item.url, '_blank', 'noopener,noreferrer')
  }
}
</script>

<template>
  <!-- Loading -->
  <div v-if="isLoading" class="flex items-center justify-center h-64">
    <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin mr-2" />
    <span>Loading latest series...</span>
  </div>

  <!-- Empty -->
  <div v-else-if="items.length === 0" class="flex items-center justify-center h-64">
    <div class="text-center">
      <p class="text-lg font-semibold">No series found</p>
      <p class="text-sm text-muted">Try adjusting your search or source filter</p>
    </div>
  </div>

  <!-- Grid -->
  <div v-else class="w-full">
    <div class="flex flex-wrap gap-4" style="justify-content: space-evenly">
      <div
        v-for="(item, index) in items"
        :key="`${item.id}-${index}`"
        :class="['relative rounded-md shadow group transition-all duration-200 cursor-pointer', cardWidth]"
        :style="{
          aspectRatio: '4/6',
          ...(getFetchDateRingColor(item.fetchDate) ? {
            border: `1.5px solid #${getFetchDateRingColor(item.fetchDate)}`,
            borderRadius: '6px',
          } : {})
        }"
        @click="handleCardClick(item)"
      >
        <UTooltip :delay-open="2000" class="w-full h-full">
          <div class="relative w-full h-full rounded-md overflow-hidden hover:scale-105 transition-transform">
            <img
              :src="item.thumbnailUrl || '/kaizoku.net.png'"
              :alt="item.title"
              class="rounded-md object-cover w-full h-full"
              loading="lazy"
              @error="onImgError"
            />
            <!-- Provider Badge -->
            <div class="absolute top-1 left-1">
              <UBadge color="neutral" variant="solid" class="bg-black/70 text-white text-xs">
                {{ item.provider }}
              </UBadge>
            </div>
            <!-- Last Chapter Badge -->
            <div v-if="item.latestChapter" class="absolute -top-1 -right-1">
              <UBadge color="primary" size="xs">{{ item.latestChapter }}</UBadge>
            </div>
            <!-- In Library Heart -->
            <div v-if="item.inLibrary !== InLibraryStatus.NotInLibrary" class="absolute top-7 right-1">
              <UIcon
                name="i-lucide-heart"
                :class="[
                  'size-8 drop-shadow-sm',
                  item.inLibrary === InLibraryStatus.InLibrary ? 'text-red-500' : 'text-yellow-500'
                ]"
              />
            </div>
            <!-- Title Bar -->
            <div :class="['absolute bottom-0 left-0 w-full bg-black/60 text-white font-semibold px-2 py-1 rounded-b-md text-center truncate', textSize]">
              {{ item.title }}
            </div>
          </div>
          <template #content>
            <div class="max-w-xl min-w-[22rem] p-4 space-y-2">
              <div class="flex items-center gap-2">
                <UBadge v-if="item.latestChapter" variant="subtle">{{ item.latestChapter }}</UBadge>
                <h3 class="font-semibold text-base text-primary truncate">{{ item.title }}</h3>
              </div>
              <div v-if="item.author || item.artist" class="text-sm text-muted">
                <span v-if="item.author">by {{ item.author }}</span>
                <span v-if="item.artist && item.artist !== item.author"> &middot; art by {{ item.artist }}</span>
              </div>
              <div v-if="item.genre?.length" class="flex flex-wrap gap-1">
                <UBadge v-for="g in item.genre" :key="g" variant="outline" size="xs">{{ g }}</UBadge>
              </div>
              <p class="text-sm text-muted line-clamp-4">{{ item.description || 'No description available' }}</p>
              <div class="flex flex-wrap gap-1 pt-1">
                <span class="inline-flex items-center gap-1 bg-muted rounded px-2 py-0.5 text-sm font-medium">
                  {{ item.provider }} &middot; {{ item.language?.toUpperCase() }}
                </span>
              </div>
            </div>
          </template>
        </UTooltip>
      </div>
    </div>

    <!-- Load More Trigger -->
    <div v-if="hasMore" ref="loadMoreRef" class="flex items-center justify-center py-4 mt-6 min-h-[80px]">
      <div v-if="isLoadingMore" class="flex items-center gap-2">
        <UIcon name="i-lucide-loader-circle" class="size-4 animate-spin" />
        <span class="text-sm">Loading more...</span>
      </div>
      <div v-else class="text-xs text-muted">Scroll to load more</div>
    </div>

    <div v-if="!hasMore && items.length > 0" class="flex items-center justify-center py-4 mt-6">
      <p class="text-sm text-muted">No more results</p>
    </div>
  </div>
</template>
