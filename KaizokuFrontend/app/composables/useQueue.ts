import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { queueService } from '~/services/queueService'
import { JobType, ProgressStatus } from '~/types'
import type { ProgressState, DownloadCardInfo } from '~/types'

export function useQueue() {
  return useQuery({
    queryKey: ['queue'],
    queryFn: () => queueService.getQueueItems(),
    refetchInterval: 5000,
  })
}

export function useRemoveFromQueue() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => queueService.removeFromQueue(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['queue'] })
    },
  })
}

export function useClearQueue() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => queueService.clearQueue(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['queue'] })
    },
  })
}

interface DownloadProgress {
  id: string
  cardInfo: DownloadCardInfo
  percentage: number
  message: string
  status: ProgressStatus
  errorMessage?: string
}

export function useDownloadProgress() {
  const downloads = ref<Record<string, DownloadProgress>>({})
  let unsubscribe: (() => void) | null = null
  let queryClient: QueryClient | null = null
  try { queryClient = useQueryClient() } catch { /* not in query context */ }

  onMounted(async () => {
    try {
      const { getProgressHub } = await import('~/utils/signalr/progressHub')
      await getProgressHub().startConnection()

      const progressListener = (progress: ProgressState) => {
        if (progress.jobType === JobType.Download) {
          if (
            progress.progressStatus === ProgressStatus.Completed
            || progress.progressStatus === ProgressStatus.Failed
          ) {
            const { [progress.id]: _removed, ...rest } = downloads.value
            downloads.value = rest
            // Invalidate download queries so completed/failed/scheduled lists refresh
            if (queryClient) {
              queryClient.invalidateQueries({ queryKey: ['downloads'] })
            }
            return
          }

          const current = downloads.value[progress.id]
          let cardInfo = current?.cardInfo
          if (progress.parameter && !cardInfo) {
            cardInfo = progress.parameter as DownloadCardInfo
          }

          if (!cardInfo) return

          downloads.value = {
            ...downloads.value,
            [progress.id]: {
              id: progress.id,
              cardInfo,
              percentage: progress.percentage,
              message: progress.message,
              status: progress.progressStatus,
              errorMessage: progress.errorMessage,
            },
          }
        }
      }

      unsubscribe = getProgressHub().onProgress(progressListener)
    }
    catch (error) {
      console.error('Failed to setup SignalR connection for downloads:', error)
    }
  })

  onUnmounted(() => {
    unsubscribe?.()
  })

  const downloadsList = computed(() => Object.values(downloads.value))

  return {
    downloads: downloadsList,
    downloadCount: computed(() => downloadsList.value.length),
  }
}
