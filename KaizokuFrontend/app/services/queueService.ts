import { apiClient } from '~/utils/api-client'

export interface QueueItem {
  id: string
  jobType: number
  status: string
  progress?: number
  message?: string
}

export const queueService = {
  async getQueueItems(): Promise<QueueItem[]> {
    return apiClient.get<QueueItem[]>('/api/queue')
  },

  async removeFromQueue(id: string): Promise<void> {
    return apiClient.delete<void>(`/api/queue/${id}`)
  },

  async clearQueue(): Promise<void> {
    return apiClient.delete<void>('/api/queue')
  },
}
