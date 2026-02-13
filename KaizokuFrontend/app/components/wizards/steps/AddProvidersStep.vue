<script setup lang="ts">
const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
}>()

onMounted(() => {
  props.setIsLoading(false)
  props.setError(null)
  props.setCanProgress(false)
})

function handleExtensionsChange(count: number) {
  props.setCanProgress(count > 0)
}
</script>

<template>
  <div class="space-y-4">
    <p class="text-sm text-muted">
      Install sources to search and download manga from. You need at least one source installed to continue.
    </p>
    <div>
      <ProvidersProviderManager
        :is-compact="true"
        :show-search="true"
        :show-nsfw-indicator="true"
        installed-title="Installed Sources"
        available-title="Available Sources"
        @extensions-change="handleExtensionsChange"
      />
    </div>
  </div>
</template>
