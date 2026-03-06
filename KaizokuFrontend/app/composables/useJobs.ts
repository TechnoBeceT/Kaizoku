import { useQuery } from '@tanstack/vue-query'
import { jobsService } from '~/services/jobsService'
import type { JobsStatusResponse } from '~/types'

export function useJobStatus() {
  return useQuery<JobsStatusResponse>({
    queryKey: ['jobs', 'status'],
    queryFn: () => jobsService.getJobStatus(),
    staleTime: 2 * 1000,
    refetchInterval: 3 * 1000,
  })
}
