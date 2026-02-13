import { apiClient } from '~/utils/api-client'
import type {
  ReportingOverview,
  SourceStats,
  SourceEventList,
  TimelineBucket,
} from '~/types'

export const reportingService = {
  async getOverview(period: string): Promise<ReportingOverview> {
    return apiClient.get<ReportingOverview>(`/api/reporting/overview?period=${period}`)
  },

  async getSources(period: string, sort?: string): Promise<SourceStats[]> {
    const params = new URLSearchParams({ period })
    if (sort) params.append('sort', sort)
    return apiClient.get<SourceStats[]>(`/api/reporting/sources?${params.toString()}`)
  },

  async getSourceEvents(
    sourceId: string,
    options?: { limit?: number, offset?: number, eventType?: string, status?: string },
  ): Promise<SourceEventList> {
    const params = new URLSearchParams()
    if (options?.limit) params.append('limit', options.limit.toString())
    if (options?.offset) params.append('offset', options.offset.toString())
    if (options?.eventType) params.append('eventType', options.eventType)
    if (options?.status) params.append('status', options.status)
    return apiClient.get<SourceEventList>(`/api/reporting/source/${sourceId}/events?${params.toString()}`)
  },

  async getSourceTimeline(sourceId: string, bucket: string, period: string): Promise<TimelineBucket[]> {
    return apiClient.get<TimelineBucket[]>(`/api/reporting/source/${sourceId}/timeline?bucket=${bucket}&period=${period}`)
  },
}
