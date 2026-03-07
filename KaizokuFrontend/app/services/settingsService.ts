import { apiClient } from '~/utils/api-client'
import type { Settings } from '~/types'

export const settingsService = {
  async getSettings(): Promise<Settings> {
    return apiClient.get<Settings>('/api/settings')
  },

  async getAvailableLanguages(): Promise<string[]> {
    return apiClient.get<string[]>('/api/settings/languages')
  },

  async updateSettings(settings: Settings): Promise<void> {
    return apiClient.put<void>('/api/settings', settings)
  },

  async testKomga(url: string, username: string, password: string): Promise<{ success: boolean; error?: string; libraries?: Array<{ id: string; name: string }> }> {
    return apiClient.post('/api/settings/komga-test', { url, username, password })
  },
}
