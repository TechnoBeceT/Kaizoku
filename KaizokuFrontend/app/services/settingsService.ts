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
}
