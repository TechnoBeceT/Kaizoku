import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { providerService } from '~/services/providerService'
import type { Provider, ProviderPreferences } from '~/types'

export function useProviders() {
  return useQuery<Provider[]>({
    queryKey: ['providers'],
    queryFn: () => providerService.getProviders(),
    staleTime: 2 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
  })
}

export function useInstallProvider() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (pkgName: string) => providerService.installProvider(pkgName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    },
  })
}

export function useInstallProviderFromFile() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (file: File) => providerService.installProviderFromFile(file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    },
  })
}

export function useUninstallProvider() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (pkgName: string) => providerService.uninstallProvider(pkgName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    },
  })
}

export function useProviderPreferences(pkgName: MaybeRef<string>) {
  return useQuery<ProviderPreferences>({
    queryKey: ['provider-preferences', pkgName],
    queryFn: () => providerService.getProviderPreferences(toValue(pkgName)),
    enabled: () => !!toValue(pkgName),
    staleTime: 5 * 60 * 1000,
  })
}

export function useSetProviderPreferences() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (preferences: ProviderPreferences) =>
      providerService.setProviderPreferences(preferences),
    onSuccess: (_data, preferences) => {
      queryClient.invalidateQueries({
        queryKey: ['provider-preferences', preferences.apkName],
      })
    },
  })
}
