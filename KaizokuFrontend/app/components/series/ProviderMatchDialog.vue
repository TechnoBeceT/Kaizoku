<script setup lang="ts">
import type { ProviderMatchChapter, MatchInfo } from '~/types'

const props = defineProps<{
  providerId: string
}>()

const emit = defineEmits<{
  close: []
  matched: []
}>()

const toast = useToast()
const enabled = ref(true)
const { data: matchData, isLoading } = useProviderMatch(computed(() => props.providerId), enabled)
const setMatchMutation = useSetProviderMatch()

// Local copy of chapters for editing assignments
const localChapters = ref<ProviderMatchChapter[]>([])

watch(() => matchData.value, (data) => {
  if (data?.chapters) {
    localChapters.value = data.chapters.map(ch => ({ ...ch }))
  }
}, { immediate: true })

const matchInfoOptions = computed(() => {
  if (!matchData.value?.matchInfos) return []
  return matchData.value.matchInfos.map((info: MatchInfo) => ({
    label: `${info.provider}${info.scanlator && info.scanlator !== info.provider ? ` - ${info.scanlator}` : ''} [${info.language}]`,
    value: info.id,
  }))
})

const hasKnownProviders = computed(() => (matchInfoOptions.value?.length ?? 0) > 0)

const assignedCount = computed(() =>
  localChapters.value.filter(ch => ch.matchInfoId).length
)

function fillAll(matchInfoId: string) {
  localChapters.value = localChapters.value.map(ch => ({
    ...ch,
    matchInfoId: ch.matchInfoId || matchInfoId,
  }))
}

function clearAll() {
  localChapters.value = localChapters.value.map(ch => ({
    ...ch,
    matchInfoId: undefined,
  }))
}

async function handleSave() {
  if (!matchData.value) return
  try {
    const result = await setMatchMutation.mutateAsync({
      id: matchData.value.id,
      matchInfos: matchData.value.matchInfos,
      chapters: localChapters.value,
    })
    if (result.redownloads > 0) {
      toast.add({
        title: `Matched ${result.matched} chapters, ${result.redownloads} queued for re-download (page mismatch)`,
        color: 'warning',
      })
    } else {
      toast.add({
        title: `Matched ${result.matched} chapters successfully`,
        color: 'success',
      })
    }
    emit('matched')
    emit('close')
  } catch {
    toast.add({ title: 'Failed to apply match', color: 'error' })
  }
}

function formatChapterNumber(num?: number): string {
  if (num === undefined || num === null) return '?'
  return Number.isInteger(num) ? String(num) : num.toFixed(1)
}
</script>

<template>
  <UModal :open="true" :ui="{ content: 'sm:max-w-4xl max-h-[90vh]' }" @close="emit('close')">
    <template #header>
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-link" class="size-5" />
        <h3 class="text-lg font-semibold">Match Unknown Provider</h3>
      </div>
    </template>

    <template #body>
      <div class="space-y-4 p-4 overflow-y-auto max-h-[75vh]">
        <div v-if="isLoading" class="flex items-center justify-center py-12">
          <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin text-muted" />
        </div>

        <template v-else-if="matchData">
          <!-- No known providers to match against -->
          <div v-if="!hasKnownProviders" class="text-center text-muted py-8">
            <UIcon name="i-lucide-unlink" class="size-12 mx-auto mb-4 opacity-50" />
            <p>No known providers available to match against.</p>
            <p class="text-sm mt-1">Add a source to this series first, then try matching.</p>
          </div>

          <template v-else>
            <!-- Fill All controls -->
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm font-medium">Assign all unmatched to:</span>
              <UButton
                v-for="info in matchInfoOptions"
                :key="info.value"
                size="xs"
                variant="soft"
                @click="fillAll(info.value)"
              >
                {{ info.label }}
              </UButton>
              <UButton
                size="xs"
                variant="ghost"
                color="error"
                icon="i-lucide-x"
                label="Clear All"
                @click="clearAll"
              />
              <span class="ml-auto text-sm text-muted">
                {{ assignedCount }}/{{ localChapters.length }} assigned
              </span>
            </div>

            <!-- Chapter list -->
            <div class="border border-default rounded-lg overflow-hidden">
              <table class="w-full text-sm">
                <thead class="bg-elevated border-b border-default">
                  <tr>
                    <th class="text-left px-3 py-2 font-medium">Ch.</th>
                    <th class="text-left px-3 py-2 font-medium">Filename</th>
                    <th class="text-left px-3 py-2 font-medium">Pages</th>
                    <th class="text-left px-3 py-2 font-medium min-w-[200px]">Assign To</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(chapter, idx) in localChapters"
                    :key="idx"
                    class="border-b border-default last:border-0"
                    :class="{ 'bg-warning/5': chapter.pageMismatch }"
                  >
                    <td class="px-3 py-2 font-mono">
                      {{ formatChapterNumber(chapter.chapterNumber) }}
                    </td>
                    <td class="px-3 py-2 truncate max-w-[300px]" :title="chapter.filename">
                      {{ chapter.filename }}
                    </td>
                    <td class="px-3 py-2">
                      <span v-if="chapter.localPages" :class="{ 'text-warning font-medium': chapter.pageMismatch }">
                        {{ chapter.localPages }}
                        <span v-if="chapter.sourcePages" class="text-muted">/{{ chapter.sourcePages }}</span>
                        <UIcon v-if="chapter.pageMismatch" name="i-lucide-alert-triangle" class="size-3.5 text-warning inline ml-1" />
                      </span>
                    </td>
                    <td class="px-3 py-2">
                      <USelectMenu
                        :model-value="chapter.matchInfoId || ''"
                        :items="[{ label: 'Unassigned', value: '' }, ...matchInfoOptions]"
                        value-key="value"
                        label-key="label"
                        size="xs"
                        @update:model-value="localChapters[idx].matchInfoId = $event || undefined"
                      />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
        </template>

        <div v-else class="text-center text-muted py-8">
          <p>Failed to load match data.</p>
        </div>

        <!-- Footer -->
        <div class="flex justify-end gap-2 pt-2">
          <UButton variant="ghost" label="Cancel" @click="emit('close')" />
          <UButton
            label="Apply Match"
            icon="i-lucide-check"
            :loading="setMatchMutation.isPending.value"
            :disabled="isLoading || !hasKnownProviders || assignedCount === 0"
            @click="handleSave"
          />
        </div>
      </div>
    </template>
  </UModal>
</template>
