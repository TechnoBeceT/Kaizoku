import { apiClient } from '~/utils/api-client'
import type { JobsStatusResponse } from '~/types'

export const jobsService = {
  async getJobStatus(): Promise<JobsStatusResponse> {
    return apiClient.get<JobsStatusResponse>('/api/jobs/status')
  },
}
