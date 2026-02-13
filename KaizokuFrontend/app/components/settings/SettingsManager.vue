<script setup lang="ts">
import type { Settings } from '~/types'
import { useQueryClient } from '@tanstack/vue-query'
import { langToFlagClass } from '~/utils/language-country-map'

const props = withDefaults(defineProps<{
  sections?: string[]
  showSaveButton?: boolean
  showHeader?: boolean
  title?: string
  description?: string
  autoSave?: boolean
}>(), {
  sections: undefined,
  showSaveButton: true,
  showHeader: true,
  title: 'Settings',
  description: 'Configure your Kaizoku application settings',
  autoSave: false,
})

const emit = defineEmits<{
  save: [settings: Settings]
}>()

const toast = useToast()
const queryClient = useQueryClient()
const { data: serverSettings, isLoading: settingsLoading } = useSettings()
const { data: availableLanguages } = useAvailableLanguages()
const updateMutation = useUpdateSettings()

const localSettings = ref<Settings | null>(null)
const newRepository = ref('')
const newCategory = ref('')

// Initialize from server settings.
// In autoSave mode, only set once (don't overwrite user's in-progress edits).
// In manual mode, keep synced with server changes.
watch(serverSettings, (settings) => {
  if (!settings) return
  if (props.autoSave && localSettings.value) return
  localSettings.value = { ...settings }
}, { immediate: true })

const isLoading = computed(() => settingsLoading.value || !localSettings.value)

// Auto-save: debounced backend save + optimistic cache update
let saveTimeout: ReturnType<typeof setTimeout> | null = null

function notifyChange() {
  if (!localSettings.value || !props.autoSave) return
  // Optimistically update the query cache so re-mounts see latest values
  queryClient.setQueryData(['settings'], { ...localSettings.value })
  // Debounced save to backend
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    if (localSettings.value) {
      updateMutation.mutate(localSettings.value)
    }
  }, 500)
}

onBeforeUnmount(() => {
  if (saveTimeout && props.autoSave && localSettings.value) {
    clearTimeout(saveTimeout)
    updateMutation.mutate(localSettings.value)
  }
})

// Helpers
const timeSpanToTimeInput = (ts: string) => {
  if (!ts) return '00:00'
  let part = ts
  const dots = ts.split('.')
  if (dots.length === 2 && dots[1]) part = dots[1]
  const [h = 0, m = 0] = part.split(':').map(p => parseInt(p) || 0)
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}

const timeInputToTimeSpan = (ti: string) => {
  if (!ti) return '00:00:00'
  const [h = 0, m = 0] = ti.split(':').map(p => parseInt(p) || 0)
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:00`
}

const timeSpanToTimeInputSeconds = (ts: string) => {
  if (!ts) return '00:00:00'
  let part = ts
  const dots = ts.split('.')
  if (dots.length === 2 && dots[1]) part = dots[1]
  const [h = 0, m = 0, s = 0] = part.split(':').map(p => parseInt(p) || 0)
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const timeInputToTimeSpanSeconds = (ti: string) => {
  if (!ti) return '00:00:00'
  const [h = 0, m = 0, s = 0] = ti.split(':').map(p => parseInt(p) || 0)
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const isValidUrl = (url: string) => {
  try { new URL(url); return true } catch { return false }
}

// Language management
const availableLanguagesToAdd = computed(() =>
  (availableLanguages.value || []).filter(
    lang => !(localSettings.value?.preferredLanguages || []).includes(lang)
  )
)

function addLanguage(lang: string) {
  if (!localSettings.value || !lang || localSettings.value.preferredLanguages?.includes(lang)) return
  localSettings.value = {
    ...localSettings.value,
    preferredLanguages: [...(localSettings.value.preferredLanguages || []), lang],
  }
  notifyChange()
}

function removeLanguage(lang: string) {
  if (!localSettings.value) return
  localSettings.value = {
    ...localSettings.value,
    preferredLanguages: (localSettings.value.preferredLanguages || []).filter(l => l !== lang),
  }
  notifyChange()
}

// Repository management
function addRepository() {
  if (!newRepository.value || !isValidUrl(newRepository.value) || !localSettings.value) return
  if (localSettings.value.mihonRepositories?.includes(newRepository.value)) return
  localSettings.value = {
    ...localSettings.value,
    mihonRepositories: [...(localSettings.value.mihonRepositories || []), newRepository.value],
  }
  newRepository.value = ''
  notifyChange()
}

function removeRepository(repo: string) {
  if (!localSettings.value) return
  localSettings.value = {
    ...localSettings.value,
    mihonRepositories: (localSettings.value.mihonRepositories || []).filter(r => r !== repo),
  }
  notifyChange()
}

// Category management
function addCategory() {
  if (!newCategory.value || !localSettings.value) return
  if (localSettings.value.categories?.includes(newCategory.value)) return
  localSettings.value = {
    ...localSettings.value,
    categories: [...(localSettings.value.categories || []), newCategory.value],
  }
  newCategory.value = ''
  notifyChange()
}

function removeCategory(cat: string) {
  if (!localSettings.value) return
  localSettings.value = {
    ...localSettings.value,
    categories: (localSettings.value.categories || []).filter(c => c !== cat),
  }
  notifyChange()
}

// Save (manual mode)
async function handleSave() {
  if (!localSettings.value) return
  emit('save', localSettings.value)
  try {
    await updateMutation.mutateAsync(localSettings.value)
    toast.add({ title: 'Settings saved successfully', color: 'success' })
  } catch {
    toast.add({ title: 'Failed to save settings', color: 'error' })
  }
}

// Section visibility
const SECTION_IDS = ['content-preferences', 'mihon-repositories', 'download-settings', 'schedule-tasks', 'storage', 'flaresolverr'] as const
const showSection = (id: string) => !props.sections || props.sections.includes(id)
</script>

<template>
  <div class="space-y-6">
    <template v-if="isLoading">
      <div v-if="showHeader">
        <p class="text-muted">{{ description }}</p>
      </div>
      <div class="flex items-center justify-center py-12 text-muted">Loading settings...</div>
    </template>

    <template v-else-if="localSettings">
      <!-- Header -->
      <div v-if="showHeader" class="flex items-center justify-between">
        <p class="text-muted">{{ description }}</p>
        <UButton v-if="showSaveButton" icon="i-lucide-save" label="Save Settings" :loading="updateMutation.isPending.value" @click="handleSave" />
      </div>

      <div class="grid gap-6">
        <!-- Content Preferences -->
        <UCard v-if="showSection('content-preferences')">
          <template #header>
            <div>
              <h3 class="font-semibold">Content Preferences</h3>
              <p class="text-sm text-muted">Select your preferred languages.</p>
            </div>
          </template>
          <div class="space-y-4">
            <div class="flex flex-wrap gap-2">
              <UBadge
                v-for="lang in (localSettings.preferredLanguages || [])"
                :key="lang"
                variant="subtle"
                class="flex items-center gap-1"
              >
                <UIcon name="i-lucide-grip-vertical" class="size-3 text-muted cursor-move" />
                <span :class="langToFlagClass(lang)" class="!w-4 !h-3" />
                {{ lang }}
                <button class="ml-1 hover:text-error" @click="removeLanguage(lang)">
                  <UIcon name="i-lucide-x" class="size-3" />
                </button>
              </UBadge>
            </div>
            <div v-if="availableLanguagesToAdd.length > 0" class="space-y-2">
              <label class="text-sm font-medium">Available languages (Derived from your installed sources):</label>
              <div class="flex flex-wrap gap-1 max-h-40 overflow-y-auto">
                <UBadge
                  v-for="lang in availableLanguagesToAdd"
                  :key="lang"
                  variant="outline"
                  class="cursor-pointer hover:bg-primary hover:text-white"
                  @click="addLanguage(lang)"
                >
                  <span :class="langToFlagClass(lang)" class="!w-3.5 !h-2.5" />
                  {{ lang }}
                </UBadge>
              </div>
            </div>
          </div>
        </UCard>

        <!-- Mihon Repositories -->
        <UCard v-if="showSection('mihon-repositories')">
          <template #header>
            <div>
              <h3 class="font-semibold">Mihon Repositories</h3>
              <p class="text-sm text-muted">Configure external repositories for additional sources.</p>
            </div>
          </template>
          <div class="space-y-4">
            <div v-for="(repo, idx) in (localSettings.mihonRepositories || [])" :key="idx" class="flex items-center gap-2">
              <UInput :model-value="repo" readonly class="flex-1" />
              <UButton variant="outline" size="sm" icon="i-lucide-x" @click="removeRepository(repo)" />
            </div>
            <div class="flex items-center gap-2">
              <UInput v-model="newRepository" placeholder="Enter repository URL" class="flex-1" />
              <UButton icon="i-lucide-plus" :disabled="!newRepository || !isValidUrl(newRepository)" @click="addRepository" />
            </div>
          </div>
        </UCard>

        <!-- Download Settings -->
        <UCard v-if="showSection('download-settings')">
          <template #header>
            <div>
              <h3 class="font-semibold">Download Settings</h3>
              <p class="text-sm text-muted">Configure download behavior and limits.</p>
            </div>
          </template>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="text-sm font-medium">Number of Simultaneous Downloads</label>
              <UInput type="number" :min="1" :max="20" :model-value="localSettings.numberOfSimultaneousDownloads" @update:model-value="localSettings!.numberOfSimultaneousDownloads = parseInt($event as any) || 1; notifyChange()" />
              <p class="text-sm text-muted mt-1">Maximum number of downloads that can run simultaneously</p>
            </div>
            <div>
              <label class="text-sm font-medium">Downloads Per Source</label>
              <UInput type="number" :min="1" :max="10" :model-value="localSettings.numberOfSimultaneousDownloadsPerProvider" @update:model-value="localSettings!.numberOfSimultaneousDownloadsPerProvider = parseInt($event as any) || 1; notifyChange()" />
              <p class="text-sm text-muted mt-1">Maximum number of simultaneous downloads per source</p>
            </div>
            <div>
              <label class="text-sm font-medium">Number of Simultaneous Searches</label>
              <UInput type="number" :min="1" :max="20" :model-value="localSettings.numberOfSimultaneousSearches" @update:model-value="localSettings!.numberOfSimultaneousSearches = parseInt($event as any) || 1; notifyChange()" />
              <p class="text-sm text-muted mt-1">Maximum number of searches that can run simultaneously</p>
            </div>
            <div>
              <label class="text-sm font-medium">Chapter Download Retry Time</label>
              <UInput type="text" placeholder="HH:MM" :model-value="timeSpanToTimeInput(localSettings.chapterDownloadFailRetryTime)" @update:model-value="localSettings!.chapterDownloadFailRetryTime = timeInputToTimeSpan($event as string); notifyChange()" />
              <p class="text-sm text-muted mt-1">How long to wait before retrying a failed chapter download</p>
            </div>
            <div>
              <label class="text-sm font-medium">Chapter Download Max Retries</label>
              <UInput type="number" :min="0" :max="1000" :model-value="localSettings.chapterDownloadFailRetries" @update:model-value="localSettings!.chapterDownloadFailRetries = parseInt($event as any) || 0; notifyChange()" />
              <p class="text-sm text-muted mt-1">Maximum number of retry attempts for failed chapter downloads</p>
            </div>
          </div>
        </UCard>

        <!-- Schedule Tasks -->
        <UCard v-if="showSection('schedule-tasks')">
          <template #header>
            <div>
              <h3 class="font-semibold">Schedule Tasks</h3>
              <p class="text-sm text-muted">Configure automatic update schedules and timings.</p>
            </div>
          </template>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="text-sm font-medium">Per Title Update Schedule</label>
              <UInput type="text" placeholder="HH:MM" :model-value="timeSpanToTimeInput(localSettings.perTitleUpdateSchedule)" @update:model-value="localSettings!.perTitleUpdateSchedule = timeInputToTimeSpan($event as string); notifyChange()" />
              <p class="text-sm text-muted mt-1">How often to check for updates per title</p>
            </div>
            <div>
              <label class="text-sm font-medium">Per Source Update Schedule</label>
              <UInput type="text" placeholder="HH:MM" :model-value="timeSpanToTimeInput(localSettings.perSourceUpdateSchedule)" @update:model-value="localSettings!.perSourceUpdateSchedule = timeInputToTimeSpan($event as string); notifyChange()" />
              <p class="text-sm text-muted mt-1">How often to check for updates per source</p>
            </div>
            <div>
              <label class="text-sm font-medium">Extensions Update Check Schedule</label>
              <UInput type="text" placeholder="HH:MM" :model-value="timeSpanToTimeInput(localSettings.extensionsCheckForUpdateSchedule)" @update:model-value="localSettings!.extensionsCheckForUpdateSchedule = timeInputToTimeSpan($event as string); notifyChange()" />
              <p class="text-sm text-muted mt-1">How often to check for extension updates</p>
            </div>
          </div>
        </UCard>

        <!-- Storage -->
        <UCard v-if="showSection('storage')">
          <template #header>
            <div>
              <h3 class="font-semibold">Storage</h3>
              <p class="text-sm text-muted">Configure how archives are stored and organized.</p>
            </div>
          </template>
          <div class="space-y-4">
            <div>
              <label class="text-sm font-medium">Storage Folder</label>
              <UInput :model-value="localSettings.storageFolder || ''" readonly class="bg-muted" />
              <p class="text-sm text-muted mt-1">Current folder where series archives are stored</p>
            </div>
            <div class="flex items-center gap-2">
              <USwitch :model-value="localSettings.categorizedFolders" @update:model-value="localSettings!.categorizedFolders = $event; notifyChange()" />
              <label class="text-sm">Enable Categorized Folders</label>
            </div>
            <div v-if="localSettings.categorizedFolders" class="space-y-4">
              <div>
                <label class="text-sm font-medium">Categories</label>
                <p class="text-sm text-muted mb-2">Define categories for organizing series.</p>
              </div>
              <div class="flex flex-wrap gap-2">
                <UBadge v-for="cat in (localSettings.categories || [])" :key="cat" variant="subtle" class="flex items-center gap-1">
                  {{ cat }}
                  <button class="ml-1 hover:text-error" @click="removeCategory(cat)">
                    <UIcon name="i-lucide-x" class="size-3" />
                  </button>
                </UBadge>
              </div>
              <div class="flex items-center gap-2">
                <UInput v-model="newCategory" placeholder="Enter category name" class="flex-1" />
                <UButton icon="i-lucide-plus" :disabled="!newCategory" @click="addCategory" />
              </div>
            </div>
          </div>
        </UCard>

        <!-- FlareSolverr -->
        <UCard v-if="showSection('flaresolverr')">
          <template #header>
            <div>
              <h3 class="font-semibold">FlareSolverr Settings</h3>
              <p class="text-sm text-muted">Configure FlareSolverr for bypassing Cloudflare protection.</p>
            </div>
          </template>
          <div class="space-y-4">
            <div class="flex items-center gap-2">
              <USwitch :model-value="localSettings.flareSolverrEnabled" @update:model-value="localSettings!.flareSolverrEnabled = $event; notifyChange()" />
              <label class="text-sm">Enable FlareSolverr</label>
            </div>
            <div v-if="localSettings.flareSolverrEnabled" class="space-y-4 pl-6 border-l-2 border-muted">
              <div>
                <label class="text-sm font-medium">FlareSolverr URL</label>
                <UInput :model-value="localSettings.flareSolverrUrl" placeholder="http://localhost:8191" @update:model-value="localSettings!.flareSolverrUrl = $event as string; notifyChange()" />
              </div>
              <div>
                <label class="text-sm font-medium">FlareSolverr Timeout</label>
                <UInput type="text" placeholder="HH:MM:SS" :model-value="timeSpanToTimeInputSeconds(localSettings.flareSolverrTimeout)" @update:model-value="localSettings!.flareSolverrTimeout = timeInputToTimeSpanSeconds($event as string); notifyChange()" />
                <p class="text-sm text-muted mt-1">Request timeout for FlareSolverr operations</p>
              </div>
              <div>
                <label class="text-sm font-medium">Session TTL</label>
                <UInput type="text" placeholder="HH:MM" :model-value="timeSpanToTimeInput(localSettings.flareSolverrSessionTtl)" @update:model-value="localSettings!.flareSolverrSessionTtl = timeInputToTimeSpan($event as string); notifyChange()" />
                <p class="text-sm text-muted mt-1">How long FlareSolverr sessions should remain active</p>
              </div>
              <div class="flex items-center gap-2">
                <USwitch :model-value="localSettings.flareSolverrAsResponseFallback" @update:model-value="localSettings!.flareSolverrAsResponseFallback = $event; notifyChange()" />
                <label class="text-sm">Use as Response Fallback</label>
              </div>
            </div>
          </div>
        </UCard>
      </div>
    </template>
  </div>
</template>
