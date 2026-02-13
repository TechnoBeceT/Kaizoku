import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { setupWizardService } from '~/services/setupWizardService'
import type { ImportInfo, ProgressState, LinkedSeries } from '~/types'
import { type JobType, ProgressStatus } from '~/types'

export function useSetupWizardScanLocalFiles() {
  return useMutation({
    mutationFn: () => setupWizardService.scanLocalFiles(),
  })
}

export function useSetupWizardInstallExtensions() {
  return useMutation({
    mutationFn: () => setupWizardService.installAdditionalExtensions(),
  })
}

export function useSetupWizardSearchSeries() {
  return useMutation({
    mutationFn: () => setupWizardService.searchSeries(),
  })
}

export function useSetupWizardImports() {
  return useQuery({
    queryKey: ['setup-wizard', 'imports'],
    queryFn: () => setupWizardService.getImports(),
    enabled: false,
  })
}

export function useSetupWizardImportSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (imports: ImportInfo[]) => setupWizardService.importSeries(imports),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['setup-wizard', 'imports'] })
    },
  })
}

export function useSetupWizardImportTotals() {
  return useQuery({
    queryKey: ['setup-wizard', 'import-totals'],
    queryFn: () => setupWizardService.getImportTotals(),
    enabled: false,
  })
}

export function useSetupWizardImportSeriesWithOptions() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (disableDownloads: boolean) => setupWizardService.importSeriesWithOptions(disableDownloads),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['setup-wizard', 'imports'] })
    },
  })
}

export function useSetupWizardAugmentSeries() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ path, linkedSeries }: { path: string, linkedSeries: LinkedSeries[] }) =>
      setupWizardService.augmentSeries(path, linkedSeries),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['setup-wizard', 'imports'] })
    },
  })
}

export function useSetupWizardUpdateImport() {
  return useMutation({
    mutationFn: (importInfo: ImportInfo) => setupWizardService.updateImport(importInfo),
  })
}

export function useSetupWizardLookupSeries() {
  return useMutation({
    mutationFn: ({ keyword, searchSources }: { keyword: string, searchSources?: string[] }) =>
      setupWizardService.lookupSeries(keyword, searchSources),
  })
}

interface ProgressTrackingOptions {
  jobTypes: JobType[]
  onProgress?: (progress: ProgressState) => void
  onComplete?: (jobType: JobType) => void
  onError?: (error: string, jobType: JobType) => void
}

export function useSignalRProgress(options: ProgressTrackingOptions) {
  const { jobTypes, onProgress, onComplete, onError } = options

  const progressStates = ref<Record<number, ProgressState | null>>({}) as Ref<Record<number, ProgressState | null>>

  // Initialize states
  for (const jt of jobTypes) {
    progressStates.value[jt] = null
  }

  let unsubscribe: (() => void) | null = null
  let resolveReady: () => void
  const connectionReady = new Promise<void>((resolve) => {
    resolveReady = resolve
  })

  onMounted(async () => {
    try {
      const { getProgressHub } = await import('~/utils/signalr/progressHub')
      await getProgressHub().startConnection()

      const progressListener = (progress: ProgressState) => {
        if (jobTypes.includes(progress.jobType)) {
          progressStates.value[progress.jobType] = progress
          onProgress?.(progress)

          if (progress.progressStatus === ProgressStatus.Completed) {
            onComplete?.(progress.jobType)
          }
          else if (progress.progressStatus === ProgressStatus.Failed) {
            onError?.(progress.errorMessage ?? 'Unknown error', progress.jobType)
          }
        }
      }

      unsubscribe = getProgressHub().onProgress(progressListener)
      resolveReady()
    }
    catch (error) {
      console.error('Failed to setup SignalR connection:', error)
      resolveReady() // Resolve anyway so callers don't hang
    }
  })

  onUnmounted(() => {
    unsubscribe?.()
  })

  function getProgressForJob(jobType: JobType): ProgressState | null {
    return progressStates.value[jobType] ?? null
  }

  function isJobCompleted(jobType: JobType): boolean {
    return progressStates.value[jobType]?.progressStatus === ProgressStatus.Completed
  }

  function isJobFailed(jobType: JobType): boolean {
    return progressStates.value[jobType]?.progressStatus === ProgressStatus.Failed
  }

  function getJobProgress(jobType: JobType): number {
    return progressStates.value[jobType]?.percentage ?? 0
  }

  return {
    progressStates,
    connectionReady,
    getProgressForJob,
    isJobCompleted,
    isJobFailed,
    getJobProgress,
  }
}
