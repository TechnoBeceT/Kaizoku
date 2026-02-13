import { apiClient } from '~/utils/api-client'
import type { Provider, ProviderPreferences } from '~/types'

export const providerService = {
  async getProviders(): Promise<Provider[]> {
    return apiClient.get<Provider[]>('/api/provider/list')
  },

  async installProvider(pkgName: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/api/provider/install/${pkgName}`, null)
  },

  async installProviderFromFile(file: File): Promise<string> {
    const formData = new FormData()
    formData.append('file', file)
    return apiClient.post<string>('/api/provider/install/file', formData)
  },

  async uninstallProvider(pkgName: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/api/provider/uninstall/${pkgName}`, null)
  },

  async getProviderPreferences(pkgName: string): Promise<ProviderPreferences> {
    return apiClient.get<ProviderPreferences>(`/api/provider/preferences/${pkgName}`)
  },

  async setProviderPreferences(preferences: ProviderPreferences): Promise<void> {
    return apiClient.post<void>('/api/provider/preferences', preferences)
  },

  getProviderIconUrl(apkName: string): string {
    return `/api/provider/icon/${apkName}`
  },
}
