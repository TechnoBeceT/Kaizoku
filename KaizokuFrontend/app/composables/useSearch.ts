import { useQuery, useMutation } from '@tanstack/vue-query'
import { searchService, type SearchParams } from '~/services/searchService'
import type { LinkedSeries } from '~/types'

export function useAvailableSearchSources() {
  return useQuery({
    queryKey: ['search', 'sources'],
    queryFn: () => searchService.getAvailableSearchSources(),
  })
}

export function useSearchSeries(params: MaybeRef<SearchParams>, enabled?: MaybeRef<boolean>) {
  return useQuery({
    queryKey: ['search', 'series', params],
    queryFn: () => {
      const p = toValue(params)
      return searchService.searchSeries(p)
    },
    enabled: () => {
      if (enabled !== undefined) return toValue(enabled)
      const p = toValue(params)
      return !!p.keyword?.trim()
    },
  })
}

export function useAugmentSeries() {
  return useMutation({
    mutationFn: (linkedSeries: LinkedSeries[]) =>
      searchService.augmentSeries(linkedSeries),
  })
}

export function useSearch() {
  const augmentMutation = useAugmentSeries()

  const searchSeries = (params: SearchParams) => {
    return searchService.searchSeries(params)
  }

  const augmentSeries = (linkedSeries: LinkedSeries[]) => {
    return augmentMutation.mutateAsync(linkedSeries)
  }

  return {
    searchSeries,
    augmentSeries,
    isAugmenting: augmentMutation.isPending,
    augmentError: augmentMutation.error,
  }
}
