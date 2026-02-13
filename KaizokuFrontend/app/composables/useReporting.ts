import { useQuery } from '@tanstack/vue-query'
import { reportingService } from '~/services/reportingService'
import type { ReportingOverview, SourceStats, SourceEventList, TimelineBucket } from '~/types'

export function useReportingOverview(period: MaybeRef<string>) {
  return useQuery<ReportingOverview>({
    queryKey: ['reporting', 'overview', period],
    queryFn: () => reportingService.getOverview(toValue(period)),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useReportingSources(period: MaybeRef<string>, sort?: MaybeRef<string | undefined>) {
  return useQuery<SourceStats[]>({
    queryKey: ['reporting', 'sources', period, sort],
    queryFn: () => reportingService.getSources(toValue(period), toValue(sort ?? '')),
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
  })
}

export function useSourceEvents(
  sourceId: MaybeRef<string>,
  params: MaybeRef<{ limit?: number, offset?: number, eventType?: string, status?: string }>,
  enabled: MaybeRef<boolean> = true,
) {
  return useQuery<SourceEventList>({
    queryKey: ['reporting', 'events', sourceId, params],
    queryFn: () => reportingService.getSourceEvents(toValue(sourceId), toValue(params)),
    enabled: () => toValue(enabled) && !!toValue(sourceId),
    staleTime: 10 * 1000,
  })
}

export function useSourceTimeline(
  sourceId: MaybeRef<string>,
  bucket: MaybeRef<string>,
  period: MaybeRef<string>,
  enabled: MaybeRef<boolean> = true,
) {
  return useQuery<TimelineBucket[]>({
    queryKey: ['reporting', 'timeline', sourceId, bucket, period],
    queryFn: () => reportingService.getSourceTimeline(toValue(sourceId), toValue(bucket), toValue(period)),
    enabled: () => toValue(enabled) && !!toValue(sourceId),
    staleTime: 60 * 1000,
  })
}
