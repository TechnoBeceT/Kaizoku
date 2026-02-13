<script setup lang="ts">
const { searchTerm, setSearchTerm, currentPage, isSearchDisabled } = useSearchState()
const mobileMenuOpen = ref(false)

const placeholder = computed(() => {
  switch (currentPage.value) {
    case 'library': return 'Search series...'
    case 'providers': return 'Search sources...'
    case 'queue': return 'Search queue...'
    default: return 'Search...'
  }
})
</script>

<template>
  <header class="sticky top-0 z-30 flex h-14 items-center gap-4 border-b border-default bg-default px-4 sm:static sm:h-auto sm:border-0 sm:bg-transparent sm:px-6">
    <!-- Mobile menu trigger -->
    <USlideover v-model:open="mobileMenuOpen" side="left" class="sm:hidden">
      <UButton
        icon="i-lucide-panel-left"
        variant="outline"
        class="sm:hidden"
        @click="mobileMenuOpen = true"
      />
      <template #body>
        <LayoutKzkNavbar />
      </template>
    </USlideover>

    <LayoutKzkBreadcrumb />

    <div class="relative ml-auto flex-1 md:grow-0">
      <UInput
        type="search"
        :model-value="searchTerm"
        :placeholder="placeholder"
        :disabled="isSearchDisabled"
        icon="i-lucide-search"
        class="w-full md:w-[200px] lg:w-[320px]"
        @update:model-value="setSearchTerm($event as string)"
      />
    </div>

    <UButton
      :icon="$colorMode.value === 'dark' ? 'i-lucide-sun' : 'i-lucide-moon'"
      variant="ghost"
      @click="$colorMode.preference = $colorMode.value === 'dark' ? 'light' : 'dark'"
    />
  </header>
</template>
