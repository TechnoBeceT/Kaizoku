import { apiClient } from '~/utils/api-client'
import type { ImportInfo, LinkedSeries, SetupOperationResponse, ImportResponse, ImportTotals } from '~/types'

export const setupWizardService = {
  async scanLocalFiles(): Promise<SetupOperationResponse> {
    return apiClient.post<SetupOperationResponse>('/api/setup/scan')
  },

  async installAdditionalExtensions(): Promise<SetupOperationResponse> {
    return apiClient.post<SetupOperationResponse>('/api/setup/install-extensions')
  },

  async searchSeries(): Promise<SetupOperationResponse> {
    return apiClient.post<SetupOperationResponse>('/api/setup/search')
  },

  async getImports(): Promise<ImportInfo[]> {
    return apiClient.get<ImportInfo[]>('/api/setup/imports')
  },

  async importSeries(imports: ImportInfo[]): Promise<ImportResponse> {
    return apiClient.post<ImportResponse>('/api/setup/import', imports)
  },

  async getImportTotals(): Promise<ImportTotals> {
    return apiClient.get<ImportTotals>('/api/setup/imports/totals')
  },

  async importSeriesWithOptions(disableDownloads: boolean): Promise<ImportResponse> {
    return apiClient.post<ImportResponse>(`/api/setup/import?disableDownloads=${disableDownloads}`, {})
  },

  async augmentSeries(path: string, linkedSeries: LinkedSeries[]): Promise<ImportInfo> {
    return apiClient.post<ImportInfo>(`/api/setup/augment?path=${encodeURIComponent(path)}`, linkedSeries)
  },

  async updateImport(importInfo: ImportInfo): Promise<void> {
    await apiClient.post('/api/setup/update', importInfo)
  },

  async lookupSeries(keyword: string, searchSources?: string[], languages?: string): Promise<LinkedSeries[]> {
    const params = new URLSearchParams()
    params.append('keyword', keyword)
    if (languages) params.append('languages', languages)
    if (searchSources && searchSources.length > 0) {
      searchSources.forEach(source => params.append('searchSources', source))
    }
    return apiClient.get<LinkedSeries[]>(`/api/search?${params.toString()}`)
  },
}
