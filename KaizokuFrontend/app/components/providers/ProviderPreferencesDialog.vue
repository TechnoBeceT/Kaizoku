<script setup lang="ts">
import { EntryType, type ProviderPreferences, type ProviderPreference } from '~/types'
import { providerService } from '~/services/providerService'

const props = defineProps<{
  apkName: string
  providerName?: string
}>()

const emit = defineEmits<{
  close: []
}>()

// No internal open state needed — component is behind v-if in parent,
// so it's always open when mounted and removed when closed
const preferences = ref<ProviderPreferences | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref<string | null>(null)

// Load preferences when the dialog mounts
onMounted(async () => {
  if (props.apkName) {
    try {
      preferences.value = await providerService.getProviderPreferences(props.apkName)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load preferences'
    } finally {
      loading.value = false
    }
  } else {
    loading.value = false
  }
})

function handleClose() {
  emit('close')
}

async function handleSave() {
  if (!preferences.value) return
  saving.value = true
  error.value = null
  try {
    await providerService.setProviderPreferences(preferences.value)
    handleClose()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Failed to save preferences'
  } finally {
    saving.value = false
  }
}

function getCurrentValue(pref: ProviderPreference): unknown {
  return pref.currentValue ?? pref.defaultValue
}

function getComboBoxValue(pref: ProviderPreference): string {
  let val = pref.currentValue ?? pref.defaultValue
  if (val === null || val === undefined || val === '') val = pref.defaultValue
  if (pref.entryValues?.length) {
    if (pref.entryValues.includes(val as string)) return val as string
    return pref.entryValues[0] ?? ''
  }
  return (val as string) || ''
}

function getProcessedSummary(pref: ProviderPreference): string {
  if (!pref.summary) return ''
  if (pref.type === EntryType.ComboBox && pref.summary.includes('%s')) {
    const currentValue = getCurrentValue(pref) as string
    if (currentValue && pref.entries && pref.entryValues) {
      const idx = pref.entryValues.indexOf(currentValue)
      if (idx !== -1 && idx < pref.entries.length) {
        return pref.summary.replace(/%s/g, pref.entries[idx] ?? '').replace(/\n/g, '<br/>')
      }
    }
    return pref.summary.replace(/%s/g, String(currentValue ?? '')).replace(/\n/g, '<br/>')
  }
  return pref.summary.replace(/\n/g, '<br/>')
}

function updateValue(index: number, newValue: unknown) {
  if (!preferences.value) return
  preferences.value = {
    ...preferences.value,
    preferences: preferences.value.preferences.map((pref, i) =>
      i === index ? { ...pref, currentValue: newValue } : pref
    ),
  }
}
</script>

<template>
  <UModal :open="true" :ui="{ content: 'sm:max-w-7xl max-h-[90vh]' }" @close="handleClose">
    <template #body>
      <div class="space-y-4 p-4 overflow-y-auto max-h-[80vh]">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-settings" class="size-5" />
          <h3 class="text-lg font-semibold">
            {{ providerName ? `${providerName} Settings` : 'Provider Settings' }}
          </h3>
        </div>
        <p class="text-sm text-muted">Configure preferences for this provider.</p>

        <div v-if="error" class="bg-error/10 border border-error/20 rounded-md p-3">
          <p class="text-sm text-error">{{ error }}</p>
        </div>

        <div v-if="loading" class="flex items-center justify-center py-8">
          <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin" />
          <span class="ml-2">Loading preferences...</span>
        </div>

        <template v-else-if="preferences">
          <div v-if="preferences.preferences.length === 0" class="text-center text-muted py-4">
            No preferences available for this provider.
          </div>
          <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div v-for="(pref, index) in preferences.preferences" :key="index" class="flex flex-col min-h-[120px]">
              <div class="flex-1 space-y-1 mb-4">
                <label class="text-base font-medium">{{ pref.title }}</label>
                <p v-if="pref.summary" class="text-sm text-muted" v-html="getProcessedSummary(pref)" />
              </div>
              <div class="mt-auto">
                <!-- ComboBox -->
                <USelectMenu
                  v-if="pref.type === EntryType.ComboBox"
                  :model-value="getComboBoxValue(pref)"
                  :items="(pref.entries || []).map((entry, i) => ({ label: entry, value: pref.entryValues?.[i] ?? entry }))"
                  value-key="value"
                  label-key="label"
                  @update:model-value="updateValue(index, $event)"
                />
                <!-- ComboCheckBox -->
                <USelectMenu
                  v-else-if="pref.type === EntryType.ComboCheckBox"
                  :model-value="(getCurrentValue(pref) as string[]) || []"
                  :items="(pref.entries || []).map((entry, i) => ({ label: entry, value: pref.entryValues?.[i] ?? entry }))"
                  multiple
                  value-key="value"
                  label-key="label"
                  @update:model-value="updateValue(index, $event)"
                />
                <!-- TextBox -->
                <UInput
                  v-else-if="pref.type === EntryType.TextBox"
                  :model-value="(getCurrentValue(pref) as string) ?? ''"
                  placeholder="Enter value"
                  @update:model-value="updateValue(index, $event)"
                />
                <!-- Switch -->
                <div v-else-if="pref.type === EntryType.Switch" class="flex items-center gap-2">
                  <USwitch
                    :model-value="(getCurrentValue(pref) as boolean) ?? false"
                    @update:model-value="updateValue(index, $event)"
                  />
                  <span class="text-sm">{{ (getCurrentValue(pref) as boolean) ? 'Enabled' : 'Disabled' }}</span>
                </div>
                <div v-else class="text-sm text-muted">Unknown preference type</div>
              </div>
            </div>
          </div>
        </template>

        <div class="flex justify-end gap-2 pt-4">
          <UButton variant="ghost" label="Cancel" @click="handleClose" />
          <UButton
            label="Save Preferences"
            :loading="saving"
            :disabled="loading || !preferences"
            @click="handleSave"
          />
        </div>
      </div>
    </template>
  </UModal>
</template>
