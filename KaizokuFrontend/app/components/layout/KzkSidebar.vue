<script setup lang="ts">
const sidebarItems = [
  { name: 'Newly Minted', href: '/cloud-latest', icon: 'i-lucide-sparkles', topSide: true },
  { name: 'Queue', href: '/queue', icon: 'i-lucide-list', topSide: true },
  { name: 'Sources', href: '/providers', icon: 'i-lucide-plug', topSide: true },
  { name: 'Reports', href: '/reporting', icon: 'i-lucide-bar-chart-3', topSide: true },
  { name: 'Settings', href: '/settings', icon: 'i-lucide-settings', topSide: true },
]

// Export for breadcrumb use
defineExpose({ sidebarItems })
</script>

<template>
  <aside class="fixed inset-y-0 left-0 z-10 hidden w-14 flex-col border-r border-default bg-default sm:flex">
    <nav class="flex flex-col items-center gap-4 px-2 py-4">
      <NuxtLink to="/library/" class="group flex h-9 w-9 shrink-0 items-center justify-center">
        <img
          src="/kaizoku.net.png"
          alt="Kaizoku"
          width="36"
          height="36"
          class="h-9 w-9 transition-all group-hover:scale-110"
        />
      </NuxtLink>

      <UTooltip
        v-for="item in sidebarItems.filter(i => i.topSide)"
        :key="item.href"
        :text="item.name"
        :popper="{ placement: 'right' }"
      >
        <NuxtLink
          :to="item.href"
          class="flex h-9 w-9 items-center justify-center rounded-lg text-muted transition-colors hover:scale-110 hover:text-default md:h-8 md:w-8"
        >
          <UIcon :name="item.icon" class="size-6" />
        </NuxtLink>
      </UTooltip>
    </nav>

    <nav class="mt-auto flex flex-col items-center gap-4 px-2 py-4">
      <LayoutDownloadCounters />
    </nav>
  </aside>
</template>
