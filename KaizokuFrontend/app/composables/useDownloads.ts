import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { downloadsService } from '~/services/downloadsService'
import { type DownloadInfo, type DownloadInfoList, type DownloadsMetrics, QueueStatus, ErrorDownloadAction } from '~/types'

export function useDownloadsForSeries(seriesId: MaybeRef<string>) {
  return useQuery({
    queryKey: ['downloads', 'series', seriesId],
    queryFn: () => downloadsService.getDownloadsForSeries(toValue(seriesId)),
    enabled: () => !!toValue(seriesId),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useDownloads(
  status?: MaybeRef<QueueStatus | undefined>,
  limit: MaybeRef<number> = 100,
  keyword?: MaybeRef<string | undefined>,
) {
  return useQuery({
    queryKey: ['downloads', 'all', status, limit, keyword],
    queryFn: () => downloadsService.getDownloads(toValue(status), toValue(limit), toValue(keyword)),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useDownloadsByStatus(status: MaybeRef<QueueStatus>, limit: MaybeRef<number> = 100) {
  return useQuery({
    queryKey: ['downloads', 'status', status, limit],
    queryFn: () => downloadsService.getDownloadsByStatus(toValue(status), toValue(limit)),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useWaitingDownloads(limit: MaybeRef<number> = 100) {
  return useQuery({
    queryKey: ['downloads', 'waiting', limit],
    queryFn: () => downloadsService.getWaitingDownloads(toValue(limit)),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useRunningDownloads(limit: MaybeRef<number> = 100) {
  return useQuery({
    queryKey: ['downloads', 'running', limit],
    queryFn: () => downloadsService.getRunningDownloads(toValue(limit)),
    staleTime: 15 * 1000,
    refetchInterval: 30 * 1000,
  })
}

export function useCompletedDownloads(limit: MaybeRef<number> = 100) {
  return useQuery({
    queryKey: ['downloads', 'completed', limit],
    queryFn: () => downloadsService.getCompletedDownloads(toValue(limit)),
    staleTime: 5 * 60 * 1000,
    refetchInterval: 2 * 60 * 1000,
  })
}

export function useFailedDownloads(limit: MaybeRef<number> = 100) {
  return useQuery({
    queryKey: ['downloads', 'failed', limit],
    queryFn: () => downloadsService.getFailedDownloads(toValue(limit)),
    staleTime: 2 * 60 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useWaitingDownloadsWithCount(
  limit: MaybeRef<number> = 100,
  keyword?: MaybeRef<string | undefined>,
) {
  return useQuery<DownloadInfoList>({
    queryKey: ['downloads', 'waiting-with-count', limit, keyword],
    queryFn: () => downloadsService.getDownloadsByStatusWithCount(QueueStatus.WAITING, toValue(limit), toValue(keyword)),
    staleTime: 2 * 1000,
    refetchInterval: 5 * 1000,
  })
}

export function useRunningDownloadsWithCount(limit: MaybeRef<number> = 100) {
  return useQuery<DownloadInfoList>({
    queryKey: ['downloads', 'running-with-count', limit],
    queryFn: () => downloadsService.getDownloadsByStatusWithCount(QueueStatus.RUNNING, toValue(limit)),
    staleTime: 15 * 1000,
    refetchInterval: 30 * 1000,
  })
}

export function useCompletedDownloadsWithCount(
  limit: MaybeRef<number> = 100,
  keyword?: MaybeRef<string | undefined>,
) {
  return useQuery<DownloadInfoList>({
    queryKey: ['downloads', 'completed-with-count', limit, keyword],
    queryFn: () => downloadsService.getDownloadsByStatusWithCount(QueueStatus.COMPLETED, toValue(limit), toValue(keyword)),
    staleTime: 2 * 1000,
    refetchInterval: 5 * 1000,
  })
}

export function useFailedDownloadsWithCount(
  limit: MaybeRef<number> = 100,
  keyword?: MaybeRef<string | undefined>,
) {
  return useQuery<DownloadInfoList>({
    queryKey: ['downloads', 'failed-with-count', limit, keyword],
    queryFn: () => downloadsService.getDownloadsByStatusWithCount(QueueStatus.FAILED, toValue(limit), toValue(keyword)),
    staleTime: 2 * 60 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useDownloadsMetrics() {
  return useQuery<DownloadsMetrics>({
    queryKey: ['downloads', 'metrics'],
    queryFn: () => downloadsService.getDownloadsMetrics(),
    staleTime: 5 * 1000,
    refetchInterval: 10 * 1000,
    refetchIntervalInBackground: true,
  })
}

export function useManageErrorDownload() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, action }: { id: string, action: ErrorDownloadAction }) =>
      downloadsService.manageErrorDownload(id, action),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['downloads', 'failed'] })
      queryClient.invalidateQueries({ queryKey: ['downloads', 'failed-with-count'] })
      queryClient.invalidateQueries({ queryKey: ['downloads', 'metrics'] })
    },
  })
}
