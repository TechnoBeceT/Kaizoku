<script setup lang="ts">
const { data: metrics, isLoading, error } = useDownloadsMetrics()
</script>

<template>
  <NuxtLink
    v-if="!isLoading && !error && metrics"
    to="/queue"
    class="flex flex-col items-center gap-4 cursor-pointer hover:opacity-80 transition-opacity"
  >
    <UTooltip :text="`Active Downloads: ${metrics.downloads}`" :popper="{ placement: 'right' }">
      <div class="flex flex-col items-center gap-1">
        <div class="flex h-9 w-9 items-center justify-center rounded-lg text-blue-500 md:h-8 md:w-8">
          <UIcon name="i-lucide-download" class="size-6" />
        </div>
        <span class="text-xs font-medium text-blue-500 min-w-5 text-center">{{ metrics.downloads }}</span>
      </div>
    </UTooltip>

    <UTooltip :text="`Queued Downloads: ${metrics.queued}`" :popper="{ placement: 'right' }">
      <div class="flex flex-col items-center gap-1">
        <div class="flex h-9 w-9 items-center justify-center rounded-lg text-yellow-500 md:h-8 md:w-8">
          <UIcon name="i-lucide-clock" class="size-6" />
        </div>
        <span class="text-xs font-medium text-yellow-500 min-w-5 text-center">{{ metrics.queued }}</span>
      </div>
    </UTooltip>

    <UTooltip :text="`Failed Downloads: ${metrics.failed}`" :popper="{ placement: 'right' }">
      <div class="flex flex-col items-center gap-1">
        <div class="flex h-9 w-9 items-center justify-center rounded-lg text-red-500 md:h-8 md:w-8">
          <UIcon name="i-lucide-alert-triangle" class="size-6" />
        </div>
        <span class="text-xs font-medium text-red-500 min-w-5 text-center">{{ metrics.failed }}</span>
      </div>
    </UTooltip>
  </NuxtLink>
</template>
