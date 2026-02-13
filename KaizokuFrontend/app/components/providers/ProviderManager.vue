<script setup lang="ts">
import type { Provider } from '~/types'
import { providerService } from '~/services/providerService'

const emit = defineEmits<{
  extensionsChange: [count: number]
}>()

const props = withDefaults(defineProps<{
  isCompact?: boolean
  showSearch?: boolean
  showNsfwIndicator?: boolean
  installedGridCols?: string
  availableGridCols?: string
  installedTitle?: string
  availableTitle?: string
  searchTerm?: string
}>(), {
  isCompact: false,
  showSearch: true,
  showNsfwIndicator: true,
  installedGridCols: 'grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3',
  availableGridCols: 'grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3',
  installedTitle: 'Installed',
  availableTitle: 'Available',
  searchTerm: '',
})

// Local search input for filtering available extensions
const localSearchTerm = ref('')
const debouncedLocalSearch = useDebounce(localSearchTerm, 300)

const extensions = ref<Provider[]>([])
const loading = ref(true)
const actionLoading = ref<string | null>(null)
const isUploadingApk = ref(false)
const fileInputRef = ref<HTMLInputElement>()
const showPrefsFor = ref<{ apkName: string; name: string } | null>(null)

// Fetch providers with retry — Suwayomi may still be indexing repos after settings save
let retryTimer: ReturnType<typeof setTimeout> | null = null

async function fetchProviders(): Promise<Provider[]> {
  try {
    return await providerService.getProviders()
  } catch (err) {
    console.error('Failed to load extensions:', err)
    return []
  }
}

onMounted(async () => {
  extensions.value = await fetchProviders()

  // If empty, Suwayomi may still be indexing repos — retry with increasing delay.
  // Default repo extensions can take 30-60s to become available after first setup.
  if (extensions.value.length === 0) {
    let retries = 0
    const maxRetries = 20
    const poll = async () => {
      retries++
      extensions.value = await fetchProviders()
      if (extensions.value.length === 0 && retries < maxRetries) {
        // Exponential backoff: 2s, 3s, 4s, ... capped at 5s
        const delay = Math.min(2000 + retries * 500, 5000)
        retryTimer = setTimeout(poll, delay)
      } else {
        loading.value = false
      }
    }
    retryTimer = setTimeout(poll, 2000)
  } else {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  if (retryTimer) clearTimeout(retryTimer)
})

const installedExtensions = computed(() => {
  const installed = extensions.value.filter(ext => ext.installed)
  const search = (props.searchTerm?.trim() || debouncedLocalSearch.value?.trim() || '').toLowerCase()
  if (!search) return installed
  return installed.filter(ext =>
    ext.name.toLowerCase().includes(search) ||
    ext.lang.toLowerCase().includes(search)
  )
})

const availableExtensions = computed(() => {
  const available = extensions.value.filter(ext => !ext.installed)
  // Use external searchTerm prop (from header) if provided, otherwise local input
  const search = (props.searchTerm?.trim() || debouncedLocalSearch.value?.trim() || '').toLowerCase()
  if (!search) return available
  return available.filter(ext =>
    ext.name.toLowerCase().includes(search) ||
    ext.lang.toLowerCase().includes(search)
  )
})

const availableTotalCount = computed(() =>
  extensions.value.filter(ext => !ext.installed).length
)

// Notify parent when installed count changes
watch(installedExtensions, (installed) => {
  emit('extensionsChange', installed.length)
}, { immediate: true })

async function handleInstall(pkgName: string) {
  try {
    actionLoading.value = pkgName
    await providerService.installProvider(pkgName)
    extensions.value = extensions.value.map(ext =>
      ext.pkgName === pkgName ? { ...ext, installed: true } : ext
    )
    const installed = extensions.value.find(ext => ext.pkgName === pkgName)
    if (installed) {
      showPrefsFor.value = { apkName: installed.apkName, name: installed.name }
    }
  } catch (err) {
    console.error('Failed to install extension:', err)
  } finally {
    actionLoading.value = null
  }
}

async function handleUninstall(pkgName: string) {
  try {
    actionLoading.value = pkgName
    await providerService.uninstallProvider(pkgName)
    extensions.value = extensions.value.map(ext =>
      ext.pkgName === pkgName ? { ...ext, installed: false } : ext
    )
  } catch (err) {
    console.error('Failed to uninstall extension:', err)
  } finally {
    actionLoading.value = null
  }
}

function handleApkButtonClick() {
  fileInputRef.value?.click()
}

async function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || !file.name.endsWith('.apk')) return
  try {
    isUploadingApk.value = true
    const pkgName = await providerService.installProviderFromFile(file)
    if (pkgName) {
      extensions.value = await providerService.getProviders()
      const ext = extensions.value.find(e => e.pkgName === pkgName)
      if (ext) showPrefsFor.value = { apkName: ext.apkName, name: ext.name }
    }
  } catch (err) {
    console.error('Failed to install APK:', err)
  } finally {
    isUploadingApk.value = false
    input.value = ''
  }
}

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.src.endsWith('/kaizoku.net.png')) {
    img.src = '/kaizoku.net.png'
  }
}
</script>

<template>
  <div class="space-y-4">
    <div v-if="loading" class="flex items-center justify-center min-h-[200px]">
      <span class="text-muted">Loading sources...</span>
    </div>

    <template v-else>
      <!-- Installed -->
      <div v-if="installedExtensions.length > 0" class="space-y-4">
        <div class="flex justify-between items-center">
          <h2 :class="isCompact ? 'text-lg font-medium' : 'text-xl font-semibold'">{{ installedTitle }}</h2>
          <span class="text-sm text-muted">{{ installedExtensions.length }} provider{{ installedExtensions.length !== 1 ? 's' : '' }} installed</span>
        </div>
        <div :class="['grid', installedGridCols, isCompact ? 'gap-2' : 'gap-4']">
          <UCard v-for="ext in installedExtensions" :key="ext.pkgName">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3 flex-1 min-w-0">
                <div class="relative flex-shrink-0">
                  <img
                    :src="ext.iconUrl || '/kaizoku.net.png'"
                    :alt="`${ext.name} icon`"
                    :class="isCompact ? 'h-12 w-12' : 'h-20 w-20'"
                    class="rounded-lg object-cover"
                    loading="lazy"
                    @error="onImgError"
                  />
                  <div v-if="showNsfwIndicator && ext.isNsfw" class="absolute -top-1 -right-1">
                    <span class="rounded-full bg-red-500 text-white text-[9px] px-1">18+</span>
                  </div>
                </div>
                <div class="flex-1 min-w-0">
                  <h4 :class="isCompact ? 'text-sm' : 'text-lg'" class="font-semibold truncate">{{ ext.name }}</h4>
                  <div class="text-xs text-muted truncate">
                    v{{ ext.versionName }}
                    &nbsp;{{ ext.lang.toUpperCase() }}
                  </div>
                </div>
              </div>
              <div class="flex items-center gap-2 flex-shrink-0">
                <UButton
                  variant="outline"
                  size="sm"
                  icon="i-lucide-settings"
                  @click="showPrefsFor = { apkName: ext.apkName, name: ext.name }"
                />
                <UButton
                  color="error"
                  variant="outline"
                  size="sm"
                  icon="i-lucide-trash-2"
                  :label="isCompact ? 'Remove' : 'Uninstall'"
                  :loading="actionLoading === ext.pkgName"
                  @click="handleUninstall(ext.pkgName)"
                />
              </div>
            </div>
          </UCard>
        </div>
      </div>

      <UDivider v-if="installedExtensions.length > 0 && availableTotalCount > 0" />

      <!-- Search + Available -->
      <div v-if="availableTotalCount > 0" class="space-y-4">
        <div class="flex justify-between items-center">
          <h2 :class="isCompact ? 'text-lg font-medium' : 'text-xl font-semibold'">{{ availableTitle }}</h2>
          <div class="flex items-center gap-4">
            <span class="text-sm text-muted">{{ availableTotalCount }} provider{{ availableTotalCount !== 1 ? 's' : '' }} available</span>
            <UButton size="sm" icon="i-lucide-upload" :label="isUploadingApk ? 'Installing...' : 'Install From APK'" :loading="isUploadingApk" @click="handleApkButtonClick" />
          </div>
        </div>
        <UInput
          v-if="showSearch"
          v-model="localSearchTerm"
          icon="i-lucide-search"
          placeholder="Search sources by name or language..."
          class="w-full"
        />
        <div :class="['grid', availableGridCols, isCompact ? 'gap-2' : 'gap-4']">
          <UCard v-for="ext in availableExtensions" :key="ext.pkgName">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3 flex-1 min-w-0">
                <div class="relative flex-shrink-0">
                  <img
                    :src="ext.iconUrl || '/kaizoku.net.png'"
                    :alt="`${ext.name} icon`"
                    :class="isCompact ? 'h-12 w-12' : 'h-20 w-20'"
                    class="rounded-lg object-cover"
                    loading="lazy"
                    @error="onImgError"
                  />
                  <div v-if="showNsfwIndicator && ext.isNsfw" class="absolute -top-1 -right-1">
                    <span class="rounded-full bg-red-500 text-white text-[9px] px-1">18+</span>
                  </div>
                </div>
                <div class="flex-1 min-w-0">
                  <h4 :class="isCompact ? 'text-sm' : 'text-lg'" class="font-semibold truncate">{{ ext.name }}</h4>
                  <div class="text-xs text-muted truncate">
                    v{{ ext.versionName }}
                    &nbsp;{{ ext.lang.toUpperCase() }}
                  </div>
                </div>
              </div>
              <UButton
                size="sm"
                icon="i-lucide-download"
                label="Install"
                :loading="actionLoading === ext.pkgName"
                @click="handleInstall(ext.pkgName)"
              />
            </div>
          </UCard>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="installedExtensions.length === 0 && availableExtensions.length === 0" class="text-center text-muted py-8">
        <template v-if="debouncedLocalSearch?.trim()">
          No sources found matching "{{ debouncedLocalSearch }}".
        </template>
        <template v-else>No sources available.</template>
      </div>
    </template>

    <!-- Hidden file input -->
    <input ref="fileInputRef" type="file" accept=".apk" class="hidden" @change="handleFileChange" />

    <!-- Auto-open preferences -->
    <ProvidersProviderPreferencesDialog
      v-if="showPrefsFor"
      :apk-name="showPrefsFor.apkName"
      :provider-name="showPrefsFor.name"
      @close="showPrefsFor = null"
    />
  </div>
</template>
