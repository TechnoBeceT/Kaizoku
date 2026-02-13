import { apiClient } from '~/utils/api-client'
import { buildApiUrl } from '~/utils/api-config'
import type {
  Source,
  SeriesInfo,
  SeriesExtendedInfo,
  ProviderMatch,
  AugmentedResponse,
  LatestSeriesInfo,
  SearchSource,
  SeriesIntegrityResult,
  DeepVerifyResult,
} from '~/types'

export const seriesService = {
  async getSources(): Promise<Source[]> {
    return apiClient.get<Source[]>('/api/serie/source')
  },

  async getSourceIcon(apk: string): Promise<Blob> {
    const response = await fetch(buildApiUrl(`/api/serie/source/icon/${apk}`))
    return response.blob()
  },

  async getSeriesThumb(id: number): Promise<Blob> {
    const response = await fetch(buildApiUrl(`/api/serie/thumb/${id}`))
    return response.blob()
  },

  async addSeries(augmentedResponse: AugmentedResponse): Promise<{ id: string }> {
    return apiClient.post<{ id: string }>('/api/serie', augmentedResponse)
  },

  async getLibrary(): Promise<SeriesInfo[]> {
    return apiClient.get<SeriesInfo[]>('/api/serie/library')
  },

  async getSeriesById(id: string): Promise<SeriesExtendedInfo> {
    return apiClient.get<SeriesExtendedInfo>(`/api/serie?id=${id}`)
  },

  async getMatch(providerId: string): Promise<ProviderMatch | null> {
    return apiClient.get<ProviderMatch | null>(`/api/serie/match/${providerId}`)
  },

  async setMatch(providerMatch: ProviderMatch): Promise<boolean> {
    return apiClient.post<boolean>('/api/serie/match', providerMatch)
  },

  async updateSeries(seriesData: SeriesExtendedInfo): Promise<SeriesExtendedInfo> {
    return apiClient.patch<SeriesExtendedInfo>('/api/serie', seriesData)
  },

  async deleteSeries(id: string, alsoPhysical: boolean = false): Promise<void> {
    const params = new URLSearchParams({
      id,
      alsoPhysical: alsoPhysical.toString(),
    })
    return apiClient.delete<void>(`/api/serie?${params.toString()}`)
  },

  async getLatest(start: number, count: number, sourceId?: string, keyword?: string, mode?: string): Promise<LatestSeriesInfo[]> {
    const params = new URLSearchParams({
      start: start.toString(),
      count: count.toString(),
    })
    if (sourceId) params.append('sourceId', sourceId)
    if (keyword) params.append('keyword', keyword)
    if (mode && mode !== 'latest') params.append('mode', mode)
    return apiClient.get<LatestSeriesInfo[]>(`/api/serie/latest?${params.toString()}`)
  },

  async getSearchSources(): Promise<SearchSource[]> {
    return apiClient.get<SearchSource[]>('/api/search/sources')
  },

  async verifyIntegrity(id: string): Promise<SeriesIntegrityResult> {
    return apiClient.get<SeriesIntegrityResult>(`/api/serie/verify?g=${id}`)
  },

  async cleanupSeries(id: string): Promise<void> {
    return apiClient.get<void>(`/api/serie/cleanup?g=${id}`)
  },

  async deepVerify(id: string): Promise<DeepVerifyResult> {
    return apiClient.get<DeepVerifyResult>(`/api/serie/deep-verify?g=${id}`)
  },

  async verifyAll(): Promise<void> {
    return apiClient.post<void>('/api/serie/verify-all', {})
  },

  async updateAllSeries(): Promise<void> {
    return apiClient.post<void>('/api/serie/update-all', {})
  },
}
