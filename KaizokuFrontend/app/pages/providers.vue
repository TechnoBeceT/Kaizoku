<script setup lang="ts">
definePageMeta({ layout: 'default' })

const { debouncedSearchTerm } = useSearchState()
</script>

<template>
  <div class="space-y-6">
    <p class="text-muted mb-4">Manage your installed and available sources. Use the search above to filter available sources.</p>
    <ProvidersProviderManager
      :is-compact="true"
      :show-search="false"
      :show-nsfw-indicator="true"
      :search-term="debouncedSearchTerm"
      installed-title="Installed"
      available-title="Available"
    />
  </div>
</template>
