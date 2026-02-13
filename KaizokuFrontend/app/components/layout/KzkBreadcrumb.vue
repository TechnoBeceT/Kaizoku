<script setup lang="ts">
const route = useRoute()
const { seriesTitle } = useSeriesState()

const sidebarItems = [
  { name: 'Newly Minted', href: '/cloud-latest', icon: 'i-lucide-sparkles' },
  { name: 'Queue', href: '/queue', icon: 'i-lucide-list' },
  { name: 'Sources', href: '/providers', icon: 'i-lucide-plug' },
  { name: 'Settings', href: '/settings', icon: 'i-lucide-settings' },
]

const topLevelPages = ['/queue', '/cloud-latest', '/providers', '/settings']

const currentPath = computed(() => {
  const p = route.path
  return p.endsWith('/') && p !== '/' ? p.slice(0, -1) : p
})

const isTopLevelPage = computed(() => topLevelPages.includes(currentPath.value))
const isSeriesDetailPage = computed(() => currentPath.value.includes('/library/series') && !!seriesTitle.value)
const isLibraryRoot = computed(() => currentPath.value === '/' || currentPath.value === '/library')

const breadcrumbItems = computed(() => {
  if (isTopLevelPage.value) {
    const item = sidebarItems.find(i => i.href === currentPath.value)
    return [{ label: item?.name || 'Page', icon: item?.icon }]
  }

  if (isLibraryRoot.value) {
    return [{ label: 'Library', icon: 'i-lucide-library' }]
  }

  if (isSeriesDetailPage.value) {
    return [
      { label: 'Library', icon: 'i-lucide-library', to: '/library' },
      { label: seriesTitle.value },
    ]
  }

  return [{ label: 'Library', icon: 'i-lucide-library', to: '/library' }]
})
</script>

<template>
  <UBreadcrumb :items="breadcrumbItems" class="hidden md:flex" />
</template>
