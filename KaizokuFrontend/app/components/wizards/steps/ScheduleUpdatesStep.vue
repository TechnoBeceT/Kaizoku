<script setup lang="ts">
const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
}>()

const emit = defineEmits<{
  downloadOptionChange: [disabled: boolean]
}>()

const { data: imports } = useSetupWizardImports()
const disableDownloads = ref(false)

const stats = computed(() => {
  if (!imports.value) return { series: 0, providers: 0, downloads: 0 }
  const importItems = imports.value.filter(i => i.status === 0 || i.status === 1) // Import or DoNotChange
  return {
    series: importItems.length,
    providers: importItems.reduce((acc, i) => acc + (i.providers?.length || 0), 0),
    downloads: importItems.reduce((acc, i) => acc + (i.providers?.reduce((a, p) => a + (p.chapterCount || 0), 0) || 0), 0),
  }
})

onMounted(() => {
  props.setIsLoading(false)
  props.setError(null)
  props.setCanProgress(true)
})

watch(disableDownloads, (val) => {
  emit('downloadOptionChange', val)
})
</script>

<template>
  <div class="space-y-6">
    <p class="text-sm text-muted">
      Review the import summary before finalizing. You can choose to start with downloads disabled if needed.
    </p>

    <div class="grid gap-4 md:grid-cols-3">
      <UCard>
        <div class="text-center">
          <div class="text-2xl font-bold">{{ stats.series }}</div>
          <div class="text-sm text-muted">Series</div>
        </div>
      </UCard>
      <UCard>
        <div class="text-center">
          <div class="text-2xl font-bold">{{ stats.providers }}</div>
          <div class="text-sm text-muted">Providers</div>
        </div>
      </UCard>
      <UCard>
        <div class="text-center">
          <div class="text-2xl font-bold">{{ stats.downloads }}</div>
          <div class="text-sm text-muted">Scheduled Downloads</div>
        </div>
      </UCard>
    </div>

    <div class="flex items-center gap-3 p-4 rounded-lg border">
      <USwitch v-model="disableDownloads" />
      <div>
        <div class="text-sm font-medium">Start with downloads disabled</div>
        <p class="text-xs text-muted">Enable this to import series without starting downloads immediately</p>
      </div>
    </div>
  </div>
</template>
