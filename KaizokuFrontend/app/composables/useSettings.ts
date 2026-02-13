import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { settingsService } from '~/services/settingsService'
import type { Settings } from '~/types'

export function useSettings() {
  return useQuery<Settings>({
    queryKey: ['settings'],
    queryFn: () => settingsService.getSettings(),
  })
}

export function useAvailableLanguages() {
  return useQuery<string[]>({
    queryKey: ['settings', 'languages'],
    queryFn: () => settingsService.getAvailableLanguages(),
  })
}

export function useUpdateSettings() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (settings: Settings) => settingsService.updateSettings(settings),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
  })
}
