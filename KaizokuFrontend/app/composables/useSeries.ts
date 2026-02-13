import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { seriesService } from '~/services/seriesService'
import type {
  SeriesInfo,
  SeriesExtendedInfo,
  ProviderMatch,
  AugmentedResponse,
  LatestSeriesInfo,
} from '~/types'

export function useSources() {
  return useQuery({
    queryKey: ['series', 'sources'],
    queryFn: () => seriesService.getSources(),
    staleTime: 5 * 60 * 1000,
  })
}

export function useSearchSources() {
  return useQuery({
    queryKey: ['search', 'sources'],
    queryFn: () => seriesService.getSearchSources(),
    staleTime: 5 * 60 * 1000,
  })
}

export function useAddSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (augmentedResponse: AugmentedResponse) => seriesService.addSeries(augmentedResponse),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
      // When adding sources to an existing series, also refresh its detail view
      if (variables.existingSeriesId) {
        queryClient.invalidateQueries({ queryKey: ['series', 'detail', variables.existingSeriesId] })
      }
    },
  })
}

export function useLibrary() {
  return useQuery<SeriesInfo[]>({
    queryKey: ['series', 'library'],
    queryFn: () => seriesService.getLibrary(),
    staleTime: 30 * 1000,
    refetchOnWindowFocus: true,
  })
}

export function useSeriesById(id: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery<SeriesExtendedInfo>({
    queryKey: ['series', 'detail', id],
    queryFn: () => seriesService.getSeriesById(toValue(id)),
    enabled: () => toValue(enabled) && !!toValue(id),
    staleTime: 5 * 60 * 1000,
  })
}

export function useSourceIcon(apk: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery({
    queryKey: ['series', 'source', 'icon', apk],
    queryFn: () => seriesService.getSourceIcon(toValue(apk)),
    enabled: () => toValue(enabled) && !!toValue(apk),
    staleTime: 30 * 60 * 1000,
  })
}

export function useSeriesThumb(id: MaybeRef<number>, enabled: MaybeRef<boolean> = true) {
  return useQuery({
    queryKey: ['series', 'thumb', id],
    queryFn: () => seriesService.getSeriesThumb(toValue(id)),
    enabled: () => toValue(enabled) && !!toValue(id),
    staleTime: 10 * 60 * 1000,
  })
}

export function useProviderMatch(providerId: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery<ProviderMatch | null>({
    queryKey: ['series', 'match', providerId],
    queryFn: () => seriesService.getMatch(toValue(providerId)),
    enabled: () => toValue(enabled) && !!toValue(providerId),
    staleTime: 5 * 60 * 1000,
  })
}

export function useSetProviderMatch() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (providerMatch: ProviderMatch) => seriesService.setMatch(providerMatch),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['series', 'match', variables.id] })
      queryClient.invalidateQueries({ queryKey: ['series', 'detail'] })
    },
  })
}

export function useLatest(
  start: MaybeRef<number>,
  count: MaybeRef<number>,
  sourceId?: MaybeRef<string | undefined>,
  keyword?: MaybeRef<string | undefined>,
  enabled: MaybeRef<boolean> = true,
  mode?: MaybeRef<string | undefined>,
) {
  return useQuery<LatestSeriesInfo[]>({
    queryKey: ['series', 'latest', start, count, sourceId, keyword, mode],
    queryFn: () => seriesService.getLatest(toValue(start), toValue(count), toValue(sourceId), toValue(keyword), toValue(mode)),
    enabled: () => toValue(enabled),
    staleTime: 2 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
  })
}

export function useUpdateSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (seriesData: SeriesExtendedInfo) => seriesService.updateSeries(seriesData),
    onSuccess: (updatedSeries) => {
      queryClient.setQueryData(['series', 'detail', updatedSeries.id], updatedSeries)
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
    },
  })
}

export function useDeleteSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, alsoPhysical }: { id: string, alsoPhysical: boolean }) =>
      seriesService.deleteSeries(id, alsoPhysical),
    onSuccess: (_data, variables) => {
      queryClient.removeQueries({ queryKey: ['series', 'detail', variables.id] })
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
      queryClient.invalidateQueries({ queryKey: ['series', 'sources'] })
    },
  })
}

export function useVerifyIntegrity() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => seriesService.verifyIntegrity(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['series', 'detail', id] })
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
    },
  })
}

export function useDeepVerify() {
  return useMutation({
    mutationFn: (id: string) => seriesService.deepVerify(id),
  })
}

export function useCleanupSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => seriesService.cleanupSeries(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['series', 'detail', id] })
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
    },
  })
}

export function useVerifyAll() {
  return useMutation({
    mutationFn: () => seriesService.verifyAll(),
  })
}

export function useUpdateAllSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => seriesService.updateAllSeries(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['series', 'library'] })
    },
  })
}
