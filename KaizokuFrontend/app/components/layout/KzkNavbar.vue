<script setup lang="ts">
const sidebarItems = [
  { name: 'Newly Minted', href: '/cloud-latest', icon: 'i-lucide-sparkles' },
  { name: 'Queue', href: '/queue', icon: 'i-lucide-list' },
  { name: 'Sources', href: '/providers', icon: 'i-lucide-plug' },
  { name: 'Settings', href: '/settings', icon: 'i-lucide-settings' },
]
</script>

<template>
  <nav class="grid gap-6 text-lg font-medium p-4">
    <NuxtLink to="/library/" class="flex items-center gap-2">
      <img src="/kaizoku.net.png" alt="Kaizoku" width="36" height="36" class="h-9 w-9" />
      <span class="font-semibold">Kaizoku</span>
    </NuxtLink>

    <NuxtLink
      v-for="item in sidebarItems"
      :key="item.href"
      :to="item.href"
      class="flex items-center gap-4 px-2.5 text-muted hover:text-default"
    >
      <UIcon :name="item.icon" class="size-6" />
      {{ item.name }}
    </NuxtLink>

    <div class="px-2.5 mt-4">
      <LayoutDownloadCounters />
    </div>
  </nav>
</template>
