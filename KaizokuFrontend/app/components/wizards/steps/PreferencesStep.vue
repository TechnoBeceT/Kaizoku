<script setup lang="ts">
const props = defineProps<{
  setError: (error: string | null) => void
  setIsLoading: (loading: boolean) => void
  setCanProgress: (canProgress: boolean) => void
}>()

const { data: settings, isLoading: settingsLoading } = useSettings()

onMounted(() => {
  props.setError(null)
  props.setIsLoading(false)
})

// Enable progress once settings are loaded from backend
watch([settings, () => settingsLoading.value], ([s, loading]) => {
  props.setCanProgress(!!s && !loading)
}, { immediate: true })
</script>

<template>
  <div class="space-y-4">
    <p class="text-sm text-muted">
      Configure your content preferences, download settings, and other preferences.
      These settings can be changed later in the Settings page.
    </p>

    <div v-if="settingsLoading" class="flex items-center justify-center py-12 text-muted">
      Loading settings...
    </div>
    <div v-else-if="settings">
      <SettingsManager
        :sections="['content-preferences', 'mihon-repositories', 'download-settings', 'schedule-tasks', 'storage', 'flaresolverr']"
        :show-header="false"
        :show-save-button="false"
        :auto-save="true"
      />
    </div>
    <div v-else class="flex items-center justify-center py-12 text-muted">
      Unable to load settings. Make sure the backend is running.
    </div>
  </div>
</template>
