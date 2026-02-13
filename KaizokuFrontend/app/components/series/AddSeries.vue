<script setup lang="ts">
import type { AugmentedResponse, ExistingSource } from '~/types'

const props = withDefaults(defineProps<{
  title?: string
  existingSources?: ExistingSource[]
  seriesId?: string
}>(), {
  title: undefined,
  existingSources: undefined,
  seriesId: undefined,
})

const isOpen = ref(false)
const step = ref(0)
const isAugmenting = ref(false)
const isAdding = ref(false)
const augmentedData = ref<AugmentedResponse | null>(null)

const searchStepRef = ref<{ getSelectedSeries: () => any[]; hasSelection: boolean } | null>(null)

const isAddSourcesMode = computed(() => !!(props.title && props.existingSources && props.seriesId))
const dialogTitle = computed(() => {
  if (step.value === 1) return 'Configure Sources'
  return isAddSourcesMode.value ? `Add New Sources to '${props.title}'` : 'Add new series'
})

const augmentMutation = useAugmentSeries()
const addMutation = useAddSeries()

async function handleNext() {
  if (!searchStepRef.value) return
  const selected = searchStepRef.value.getSelectedSeries()
  if (selected.length === 0) return

  isAugmenting.value = true
  try {
    const augmented = await augmentMutation.mutateAsync(selected)

    // When adding sources to an existing series, set the existing series info
    if (isAddSourcesMode.value) {
      augmented.existingSeries = true
      augmented.existingSeriesId = props.seriesId
    }

    augmentedData.value = augmented
    step.value = 1
  } catch (err) {
    console.error('Failed to augment series:', err)
  } finally {
    isAugmenting.value = false
  }
}

async function handleAdd() {
  if (!augmentedData.value) return
  isAdding.value = true
  try {
    await addMutation.mutateAsync(augmentedData.value)
    handleCancel()
  } catch (err) {
    console.error('Failed to add series:', err)
  } finally {
    isAdding.value = false
  }
}

function handleCancel() {
  isOpen.value = false
  step.value = 0
  augmentedData.value = null
}

function onAugmentedUpdate(updated: AugmentedResponse) {
  augmentedData.value = updated
}
</script>

<template>
  <UModal v-model:open="isOpen" :ui="{ content: 'sm:max-w-[70%] max-h-[90vh]', body: 'p-0' }">
    <slot :open="() => { isOpen = true }">
      <UButton size="sm" icon="i-lucide-plus-circle" label="Add Series" @click="isOpen = true" />
    </slot>
    <template #body>
      <div class="flex flex-col max-h-[85vh]">
        <!-- Fixed Header -->
        <div class="shrink-0 space-y-1 p-4 pb-2">
          <h3 class="text-lg font-semibold">{{ dialogTitle }}</h3>
          <p v-if="step === 0" class="text-sm text-muted">Search for and add new series to your library.</p>
          <p v-else class="text-sm text-muted">Drag sources to set download priority. Top source is tried first.</p>
        </div>

        <!-- Scrollable Content -->
        <div class="flex-1 overflow-y-auto px-4 pb-4 min-h-[300px]">
          <SeriesSearchSeriesStep v-if="step === 0" ref="searchStepRef" />
          <SeriesConfirmSeriesStep
            v-if="step === 1 && augmentedData"
            :augmented="augmentedData"
            @update:augmented="onAugmentedUpdate"
          />
        </div>

        <!-- Fixed Footer -->
        <div class="shrink-0 flex justify-between p-4 pt-3 border-t border-default">
          <UButton variant="ghost" label="Cancel" @click="handleCancel" />
          <div class="flex gap-2">
            <UButton v-if="step > 0" variant="outline" label="Back" @click="step = 0" />
            <UButton
              v-if="step === 0"
              label="Next"
              :disabled="!searchStepRef?.hasSelection"
              :loading="isAugmenting"
              @click="handleNext"
            />
            <UButton
              v-if="step === 1"
              :label="isAddSourcesMode ? 'Add Sources' : 'Add Series'"
              :loading="isAdding"
              @click="handleAdd"
            />
          </div>
        </div>
      </div>
    </template>
  </UModal>
</template>
