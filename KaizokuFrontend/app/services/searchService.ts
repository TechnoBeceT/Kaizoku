import { apiClient } from '~/utils/api-client'
import type { LinkedSeries, AugmentedResponse, SearchSource } from '~/types'

export interface SearchParams {
  keyword: string
  languages?: string
  searchSources?: string[]
}

export const searchService = {
  async getAvailableSearchSources(): Promise<SearchSource[]> {
    return apiClient.get<SearchSource[]>('/api/search/sources')
  },

  async searchSeries(params: SearchParams): Promise<LinkedSeries[]> {
    const searchParams = new URLSearchParams({
      keyword: params.keyword,
      ...(params.languages && { languages: params.languages }),
    })
    if (params.searchSources && params.searchSources.length > 0) {
      params.searchSources.forEach(sourceId => searchParams.append('searchSources', sourceId))
    }
    return apiClient.get<LinkedSeries[]>(`/api/search?${searchParams.toString()}`)
  },

  async augmentSeries(linkedSeries: LinkedSeries[]): Promise<AugmentedResponse> {
    return apiClient.post<AugmentedResponse>('/api/search/augment', linkedSeries)
  },
}
