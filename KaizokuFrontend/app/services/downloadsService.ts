import { apiClient } from '~/utils/api-client'
import { type DownloadInfo, type DownloadInfoList, type DownloadsMetrics, QueueStatus, ErrorDownloadAction } from '~/types'

export const downloadsService = {
  async getDownloadsForSeries(seriesId: string): Promise<DownloadInfo[]> {
    return apiClient.get<DownloadInfo[]>(`/api/downloads/series?seriesId=${seriesId}`)
  },

  async getDownloads(status?: QueueStatus, limit: number = 100, keyword?: string): Promise<DownloadInfo[]> {
    const params = new URLSearchParams()
    if (status !== undefined) params.append('status', status.toString())
    params.append('limit', limit.toString())
    if (keyword) params.append('keyword', keyword)
    const queryString = params.toString()
    const url = queryString ? `/api/downloads?${queryString}` : '/api/downloads'
    const result = await apiClient.get<DownloadInfoList>(url)
    return result.downloads
  },

  async getDownloadsByStatusWithCount(status: QueueStatus, limit: number = 100, keyword?: string, offset?: number): Promise<DownloadInfoList> {
    const params = new URLSearchParams()
    params.append('status', status.toString())
    params.append('limit', limit.toString())
    if (offset !== undefined && offset > 0) params.append('offset', offset.toString())
    if (keyword) params.append('keyword', keyword)
    return apiClient.get<DownloadInfoList>(`/api/downloads?${params.toString()}`)
  },

  async getDownloadsByStatus(status: QueueStatus, limit: number = 100): Promise<DownloadInfo[]> {
    return this.getDownloads(status, limit)
  },

  async getWaitingDownloads(limit: number = 100): Promise<DownloadInfo[]> {
    return this.getDownloadsByStatus(QueueStatus.WAITING, limit)
  },

  async getRunningDownloads(limit: number = 100): Promise<DownloadInfo[]> {
    return this.getDownloadsByStatus(QueueStatus.RUNNING, limit)
  },

  async getCompletedDownloads(limit: number = 100): Promise<DownloadInfo[]> {
    return this.getDownloadsByStatus(QueueStatus.COMPLETED, limit)
  },

  async getFailedDownloads(limit: number = 100): Promise<DownloadInfo[]> {
    return this.getDownloadsByStatus(QueueStatus.FAILED, limit)
  },

  async getDownloadsMetrics(): Promise<DownloadsMetrics> {
    return apiClient.get<DownloadsMetrics>('/api/downloads/metrics')
  },

  async manageErrorDownload(id: string, action: ErrorDownloadAction): Promise<void> {
    const params = new URLSearchParams()
    params.append('id', id)
    params.append('action', action.toString())
    return apiClient.patch<void>(`/api/downloads?${params.toString()}`)
  },
}
