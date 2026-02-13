<script setup lang="ts">
import type { SourceStats, SourceEventDTO } from '~/types'

definePageMeta({ layout: 'default' })

// Period selector (persisted in sessionStorage)
const periodOptions = [
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
  { label: '30d', value: '30d' },
]
const period = useSessionStorage('reporting_period', '24h')

// Active tab
const activeTab = ref(0)
const tabs = [
  { label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { label: 'Sources', icon: 'i-lucide-server' },
  { label: 'Event Log', icon: 'i-lucide-scroll-text' },
]

// Overview data
const { data: overview, isLoading: overviewLoading } = useReportingOverview(period)

// Sources tab
const sourceSort = ref('failures')
const { data: sources, isLoading: sourcesLoading } = useReportingSources(period, sourceSort)
const expandedSource = ref<string | null>(null)

// Source detail (timeline + events for expanded source)
const timelineBucket = computed(() => period.value === '24h' ? 'hour' : 'day')
const { data: timeline } = useSourceTimeline(
  computed(() => expandedSource.value ?? ''),
  timelineBucket,
  period,
  computed(() => !!expandedSource.value),
)
const { data: sourceEvents } = useSourceEvents(
  computed(() => expandedSource.value ?? ''),
  computed(() => ({ limit: 10 })),
  computed(() => !!expandedSource.value),
)

// Event log tab
const eventLogSource = ref('')
const eventLogType = ref('')
const eventLogStatus = ref('')
const eventLogPage = ref(0)
const eventLogLimit = 25

const { data: eventLog, isLoading: eventLogLoading } = useSourceEvents(
  computed(() => eventLogSource.value || '__all__'),
  computed(() => ({
    limit: eventLogLimit,
    offset: eventLogPage.value * eventLogLimit,
    eventType: eventLogType.value || undefined,
    status: eventLogStatus.value || undefined,
  })),
  computed(() => activeTab.value === 2),
)

// Event detail modal
const selectedEvent = ref<SourceEventDTO | null>(null)
const showEventModal = computed({
  get: () => selectedEvent.value !== null,
  set: (v: boolean) => { if (!v) selectedEvent.value = null },
})

// Unique sources for event log filter
const sourceOptions = computed(() => {
  if (!sources.value) return []
  return sources.value.map(s => ({ label: `${s.sourceName} (${s.language})`, value: s.sourceId }))
})

// Helpers
function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

function formatRate(rate: number): string {
  return `${rate.toFixed(1)}%`
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function statusColor(status: string): string {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'error'
  return 'warning'
}

function categoryColor(cat: string | null): string {
  if (!cat) return 'neutral'
  const map: Record<string, string> = {
    network: 'error', timeout: 'warning', rate_limit: 'warning',
    server_error: 'error', not_found: 'neutral', parse: 'info',
    cancelled: 'neutral', unknown: 'neutral',
  }
  return map[cat] || 'neutral'
}

function rateColorClass(rate: number): string {
  if (rate >= 95) return 'bg-green-500'
  if (rate >= 80) return 'bg-yellow-500'
  if (rate >= 50) return 'bg-orange-500'
  return 'bg-red-500'
}

function toggleSource(sourceId: string) {
  expandedSource.value = expandedSource.value === sourceId ? null : sourceId
}

function maxBarValue(items: { value: number }[]): number {
  return Math.max(...items.map(i => i.value), 1)
}

// Error diagnosis — provides human-readable explanations for source failures
interface Diagnosis {
  title: string
  explanation: string
  suggestions: string[]
}

function diagnoseError(evt: SourceEventDTO): Diagnosis | null {
  if (evt.status === 'success') return null

  const cat = evt.errorCategory
  const msg = (evt.errorMessage || '').toLowerCase()
  const origin = evt.metadata?.origin ?? ''
  const eventType = evt.eventType

  // Cancelled
  if (cat === 'cancelled') {
    if (origin === 'http_request') {
      return {
        title: 'Request Cancelled',
        explanation: 'The browser cancelled this request, likely because you navigated away from the page. The source itself may still have been responding normally.',
        suggestions: ['This is not a source issue — the request was aborted client-side', 'Suwayomi may have continued processing this on the server side'],
      }
    }
    return {
      title: 'Operation Cancelled',
      explanation: 'This background operation was cancelled, possibly because the server is shutting down or a job was aborted.',
      suggestions: ['Check if the server was restarted during this time', 'If this happens frequently, the job queue may be overloaded'],
    }
  }

  // Timeout
  if (cat === 'timeout') {
    return {
      title: 'Source Timeout',
      explanation: `The source did not respond within the allowed time. This usually means the source server is overloaded or the extension is making too many sub-requests.`,
      suggestions: [
        'The source server may be experiencing high load',
        'Check if FlareSolverr is enabled — it adds significant latency per request',
        'If this source consistently times out, it may need a longer timeout or be temporarily unavailable',
        eventType === 'download' ? 'Large chapters with many pages are more prone to timeouts' : '',
      ].filter(Boolean),
    }
  }

  // Rate limit
  if (cat === 'rate_limit') {
    return {
      title: 'Rate Limited by Source',
      explanation: 'The source is blocking requests because too many were sent in a short period (HTTP 429). This is the source protecting itself from abuse.',
      suggestions: [
        'Reduce the number of simultaneous downloads/searches in Settings',
        'Wait a few minutes before retrying — the source will unblock automatically',
        'If this happens often, consider lowering "Downloads per Provider" in settings',
      ],
    }
  }

  // Server error
  if (cat === 'server_error') {
    return {
      title: 'Source Server Error',
      explanation: 'The source returned an internal server error (HTTP 5xx). This means the source itself is having problems — not a Kaizoku issue.',
      suggestions: [
        'The source server may be temporarily down for maintenance',
        'Try visiting the source website directly to verify if it\'s working',
        'This will likely resolve itself — retry later',
        msg.includes('502') || msg.includes('503') ? 'A 502/503 error usually means the source is behind a reverse proxy that can\'t reach the backend' : '',
      ].filter(Boolean),
    }
  }

  // Not found
  if (cat === 'not_found') {
    return {
      title: 'Content Not Found',
      explanation: 'The requested manga, chapter, or page was not found on the source (HTTP 404). The content may have been removed or the URL changed.',
      suggestions: [
        'The manga may have been removed from this source',
        'The source extension may need an update to handle new URL patterns',
        eventType === 'download' ? 'The chapter pages may not be available yet — Suwayomi needs to load them first' : '',
        'Check if the manga is still accessible on the source website',
      ].filter(Boolean),
    }
  }

  // Network
  if (cat === 'network') {
    const isSuwayomi = msg.includes('connection refused') || msg.includes('dial tcp') || msg.includes('localhost') || msg.includes('127.0.0.1')
    if (isSuwayomi) {
      return {
        title: 'Suwayomi Connection Failed',
        explanation: 'Kaizoku could not connect to the Suwayomi server. The embedded Suwayomi process may not be running or is still starting up.',
        suggestions: [
          'Check if Suwayomi is running (it should auto-start with Kaizoku)',
          'Suwayomi may still be initializing — wait a minute and retry',
          'If using an external Suwayomi, verify the endpoint URL in settings',
        ],
      }
    }
    return {
      title: 'Network Error',
      explanation: 'A network-level error occurred while communicating with the source. The source server may be unreachable or the connection was interrupted.',
      suggestions: [
        'Check your internet connection',
        'The source server may be temporarily unreachable',
        msg.includes('eof') || msg.includes('connection reset') ? 'The source or a proxy forcibly closed the connection — could indicate IP blocking or Cloudflare protection' : '',
        'If FlareSolverr is enabled and configured, verify it\'s reachable',
      ].filter(Boolean),
    }
  }

  // Parse error
  if (cat === 'parse') {
    return {
      title: 'Response Parse Error',
      explanation: 'The source returned data that Suwayomi couldn\'t parse. This typically means the source changed its website structure or API, and the extension needs updating.',
      suggestions: [
        'Check if an extension update is available in Sources',
        'The source website may have changed its layout/API',
        'Cloudflare protection pages can cause parse errors — check if FlareSolverr is needed',
        msg.includes('cloudflare') || msg.includes('challenge') ? 'This looks like a Cloudflare challenge — enable FlareSolverr in Settings' : '',
      ].filter(Boolean),
    }
  }

  // Unknown / fallback
  return {
    title: 'Unknown Error',
    explanation: `An unexpected error occurred during the ${eventType.replace(/_/g, ' ')} operation.`,
    suggestions: [
      'Check the error message below for details',
      'This may be a transient issue — retry the operation',
      'If this persists, the extension may have a bug',
    ],
  }
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-bold">Source Reporting</h1>
      <div class="flex items-center gap-2">
        <div class="flex rounded-md overflow-hidden border border-default">
          <button
            v-for="opt in periodOptions"
            :key="opt.value"
            :class="[
              'px-3 py-1 text-xs font-medium transition-colors',
              period === opt.value
                ? 'bg-primary text-white'
                : 'bg-default hover:bg-elevated',
            ]"
            @click="period = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="flex gap-1 border-b border-default">
      <button
        v-for="(tab, i) in tabs"
        :key="tab.label"
        class="flex items-center gap-1.5 px-3 py-2 text-sm font-medium transition-colors border-b-2"
        :class="activeTab === i ? 'border-primary text-default' : 'border-transparent text-muted hover:text-default'"
        @click="activeTab = i"
      >
        <UIcon :name="tab.icon" class="size-4" />
        {{ tab.label }}
      </button>
    </div>

    <!-- Tab: Overview -->
    <div v-if="activeTab === 0">
      <div v-if="overviewLoading" class="flex items-center justify-center py-16 text-muted">
        <UIcon name="i-lucide-loader-2" class="size-8 animate-spin" />
      </div>
      <div v-else-if="overview" class="flex flex-col gap-4">
        <!-- Stat Cards -->
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <UCard>
            <div class="text-center">
              <div class="text-2xl font-bold">{{ overview.totalEvents.toLocaleString() }}</div>
              <div class="text-xs text-muted">Total Operations</div>
            </div>
          </UCard>
          <UCard>
            <div class="text-center">
              <div class="text-2xl font-bold" :class="overview.successRate >= 90 ? 'text-green-500' : overview.successRate >= 70 ? 'text-yellow-500' : 'text-red-500'">
                {{ formatRate(overview.successRate) }}
              </div>
              <div class="text-xs text-muted">Success Rate</div>
            </div>
          </UCard>
          <UCard>
            <div class="text-center">
              <div class="text-2xl font-bold">{{ formatDuration(overview.avgDurationMs) }}</div>
              <div class="text-xs text-muted">Avg Response Time</div>
            </div>
          </UCard>
          <UCard>
            <div class="text-center">
              <div class="text-2xl font-bold">{{ overview.activeSources }}</div>
              <div class="text-xs text-muted">Active Sources</div>
            </div>
          </UCard>
        </div>

        <!-- Charts Row -->
        <div class="grid gap-4 lg:grid-cols-2">
          <!-- Slowest Sources -->
          <UCard>
            <template #header>
              <span class="text-sm font-semibold">Slowest Sources (avg)</span>
            </template>
            <div v-if="overview.slowestSources.length === 0" class="py-6 text-center text-muted text-sm">
              Not enough data yet
            </div>
            <div v-else class="flex flex-col gap-2">
              <div v-for="src in overview.slowestSources" :key="src.sourceId" class="flex items-center gap-2">
                <div class="w-28 truncate text-xs" :title="src.sourceName">{{ src.sourceName }}</div>
                <div class="flex-1 h-5 rounded bg-muted/20 overflow-hidden">
                  <div
                    class="h-full rounded bg-blue-500 transition-all duration-300"
                    :style="{ width: `${(src.avgDurationMs / maxBarValue(overview.slowestSources.map(s => ({ value: s.avgDurationMs })))) * 100}%` }"
                  />
                </div>
                <div class="w-16 text-right text-xs font-mono text-muted">{{ formatDuration(src.avgDurationMs) }}</div>
              </div>
            </div>
          </UCard>

          <!-- Most Failing Sources -->
          <UCard>
            <template #header>
              <span class="text-sm font-semibold">Most Failing Sources</span>
            </template>
            <div v-if="overview.failingSources.length === 0" class="py-6 text-center text-muted text-sm">
              No failures recorded
            </div>
            <div v-else class="flex flex-col gap-2">
              <div v-for="src in overview.failingSources" :key="src.sourceId" class="flex items-center gap-2">
                <div class="w-28 truncate text-xs" :title="src.sourceName">{{ src.sourceName }}</div>
                <div class="flex-1 h-5 rounded bg-muted/20 overflow-hidden">
                  <div
                    class="h-full rounded bg-red-500 transition-all duration-300"
                    :style="{ width: `${(src.failureCount / maxBarValue(overview.failingSources.map(s => ({ value: s.failureCount })))) * 100}%` }"
                  />
                </div>
                <div class="w-16 text-right text-xs font-mono text-muted">{{ src.failureCount }} fails</div>
              </div>
            </div>
          </UCard>
        </div>

        <!-- Recent Errors -->
        <UCard>
          <template #header>
            <span class="text-sm font-semibold">Recent Errors</span>
          </template>
          <div v-if="overview.recentErrors.length === 0" class="py-6 text-center text-muted text-sm">
            No recent errors
          </div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="border-b border-default text-left text-muted">
                  <th class="pb-2 pr-3">Source</th>
                  <th class="pb-2 pr-3">Type</th>
                  <th class="pb-2 pr-3">Category</th>
                  <th class="pb-2 pr-3">Likely Cause</th>
                  <th class="pb-2 pr-3">Error</th>
                  <th class="pb-2">Time</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="err in overview.recentErrors"
                  :key="err.id"
                  class="border-b border-default/50 cursor-pointer hover:bg-muted/10 transition-colors"
                  @click="selectedEvent = err"
                >
                  <td class="py-1.5 pr-3 font-medium">{{ err.sourceName }}</td>
                  <td class="py-1.5 pr-3">
                    <UBadge size="xs" variant="subtle" color="neutral">{{ err.eventType }}</UBadge>
                  </td>
                  <td class="py-1.5 pr-3">
                    <UBadge v-if="err.errorCategory" size="xs" variant="subtle" :color="categoryColor(err.errorCategory)">
                      {{ err.errorCategory }}
                    </UBadge>
                  </td>
                  <td class="py-1.5 pr-3 text-xs text-muted max-w-[200px] truncate">
                    {{ diagnoseError(err)?.title || '-' }}
                  </td>
                  <td class="py-1.5 pr-3 max-w-xs truncate text-muted" :title="err.errorMessage ?? ''">
                    {{ err.errorMessage || '-' }}
                  </td>
                  <td class="py-1.5 text-muted whitespace-nowrap">{{ timeAgo(err.createdAt) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </UCard>
      </div>
      <div v-else class="flex items-center justify-center py-16 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-bar-chart-3" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No reporting data available yet</p>
          <p class="text-xs mt-1">Events will appear as operations occur</p>
        </div>
      </div>
    </div>

    <!-- Tab: Source Performance -->
    <div v-if="activeTab === 1">
      <div class="mb-3 flex items-center gap-2">
        <span class="text-xs text-muted">Sort by:</span>
        <div class="flex rounded-md overflow-hidden border border-default">
          <button
            v-for="opt in [{ label: 'Failures', value: 'failures' }, { label: 'Duration', value: 'duration' }, { label: 'Events', value: 'events' }]"
            :key="opt.value"
            :class="[
              'px-3 py-1 text-xs font-medium transition-colors',
              sourceSort === opt.value
                ? 'bg-primary text-white'
                : 'bg-default hover:bg-elevated',
            ]"
            @click="sourceSort = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <div v-if="sourcesLoading" class="flex items-center justify-center py-16 text-muted">
        <UIcon name="i-lucide-loader-2" class="size-8 animate-spin" />
      </div>
      <div v-else-if="sources && sources.length > 0" class="flex flex-col gap-2">
        <UCard v-for="src in sources" :key="src.sourceId" class="cursor-pointer" @click="toggleSource(src.sourceId)">
          <!-- Source Summary Row -->
          <div class="flex items-center gap-3">
            <UIcon
              :name="expandedSource === src.sourceId ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
              class="size-4 text-muted shrink-0"
            />
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-medium text-sm truncate">{{ src.sourceName }}</span>
                <UBadge size="xs" variant="subtle" color="neutral">{{ src.language }}</UBadge>
              </div>
            </div>
            <div class="flex items-center gap-4 text-xs shrink-0">
              <div class="text-center">
                <div class="font-mono font-medium">{{ src.totalEvents }}</div>
                <div class="text-muted">events</div>
              </div>
              <div class="w-20">
                <div class="flex items-center gap-1">
                  <div class="flex-1 h-2 rounded-full bg-muted/20 overflow-hidden">
                    <div
                      class="h-full rounded-full transition-all duration-300"
                      :class="rateColorClass(src.successRate)"
                      :style="{ width: `${src.successRate}%` }"
                    />
                  </div>
                  <span class="font-mono text-xs" :class="src.successRate >= 90 ? 'text-green-500' : src.successRate >= 70 ? 'text-yellow-500' : 'text-red-500'">
                    {{ formatRate(src.successRate) }}
                  </span>
                </div>
              </div>
              <div class="text-center">
                <div class="font-mono font-medium">{{ formatDuration(src.avgDurationMs) }}</div>
                <div class="text-muted">avg</div>
              </div>
              <div class="text-center">
                <div class="font-mono font-medium" :class="src.failureCount > 0 ? 'text-red-500' : ''">{{ src.failureCount }}</div>
                <div class="text-muted">fails</div>
              </div>
            </div>
          </div>

          <!-- Expanded Detail -->
          <div v-if="expandedSource === src.sourceId" class="mt-4 border-t border-default pt-4" @click.stop>
            <div class="grid gap-4 lg:grid-cols-2">
              <!-- Timeline Chart -->
              <div>
                <h4 class="text-xs font-semibold text-muted mb-2">Timeline</h4>
                <div v-if="timeline && timeline.length > 0" class="flex items-end gap-0.5 h-24">
                  <div
                    v-for="(bucket, bi) in timeline"
                    :key="bi"
                    class="flex-1 flex flex-col justify-end h-full"
                    :title="`${bucket.timestamp}: ${bucket.successCount} ok, ${bucket.failureCount} fail`"
                  >
                    <div
                      v-if="bucket.failureCount > 0"
                      class="bg-red-500 rounded-t-sm min-h-[2px]"
                      :style="{ height: `${(bucket.failureCount / Math.max(...timeline.map(t => t.totalEvents), 1)) * 100}%` }"
                    />
                    <div
                      v-if="bucket.successCount > 0"
                      class="bg-green-500 min-h-[2px]"
                      :class="bucket.failureCount === 0 ? 'rounded-t-sm' : ''"
                      :style="{ height: `${(bucket.successCount / Math.max(...timeline.map(t => t.totalEvents), 1)) * 100}%` }"
                    />
                  </div>
                </div>
                <div v-else class="h-24 flex items-center justify-center text-xs text-muted">
                  No timeline data
                </div>
              </div>

              <!-- Event Type Breakdown -->
              <div>
                <h4 class="text-xs font-semibold text-muted mb-2">Event Types</h4>
                <div class="grid grid-cols-2 gap-2">
                  <div v-for="(bd, eventType) in src.breakdown" :key="eventType" class="rounded bg-muted/10 p-2">
                    <div class="text-xs font-medium">{{ eventType }}</div>
                    <div class="flex items-center gap-2 mt-1">
                      <span class="text-xs text-green-500">{{ bd.success }} ok</span>
                      <span v-if="bd.failed > 0" class="text-xs text-red-500">{{ bd.failed }} fail</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Recent Events for Source -->
            <div v-if="sourceEvents && sourceEvents.events.length > 0" class="mt-4">
              <h4 class="text-xs font-semibold text-muted mb-2">Recent Events</h4>
              <div class="overflow-x-auto">
                <table class="w-full text-xs">
                  <thead>
                    <tr class="border-b border-default text-left text-muted">
                      <th class="pb-1 pr-2">Type</th>
                      <th class="pb-1 pr-2">Status</th>
                      <th class="pb-1 pr-2">Duration</th>
                      <th class="pb-1 pr-2">Items</th>
                      <th class="pb-1">Time</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="evt in sourceEvents.events" :key="evt.id" class="border-b border-default/30">
                      <td class="py-1 pr-2">{{ evt.eventType }}</td>
                      <td class="py-1 pr-2">
                        <UBadge size="xs" :color="statusColor(evt.status)">{{ evt.status }}</UBadge>
                      </td>
                      <td class="py-1 pr-2 font-mono">{{ formatDuration(evt.durationMs) }}</td>
                      <td class="py-1 pr-2">{{ evt.itemsCount ?? '-' }}</td>
                      <td class="py-1 text-muted">{{ timeAgo(evt.createdAt) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </UCard>
      </div>
      <div v-else class="flex items-center justify-center py-16 text-muted">
        <div class="text-center">
          <UIcon name="i-lucide-server" class="size-12 mx-auto mb-4 opacity-50" />
          <p>No source data available</p>
        </div>
      </div>
    </div>

    <!-- Tab: Event Log -->
    <div v-if="activeTab === 2">
      <!-- Filters -->
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <USelectMenu
          v-model="eventLogSource"
          :items="[{ label: 'All Sources', value: '' }, ...sourceOptions]"
          class="w-48"
          size="xs"
          placeholder="All Sources"
          value-key="value"
        />
        <USelectMenu
          v-model="eventLogType"
          :items="[
            { label: 'All Types', value: '' },
            { label: 'Download', value: 'download' },
            { label: 'Get Chapters', value: 'get_chapters' },
            { label: 'Get Latest', value: 'get_latest' },
            { label: 'Search', value: 'search' },
            { label: 'Get Popular', value: 'get_popular' },
          ]"
          class="w-40"
          size="xs"
          placeholder="All Types"
          value-key="value"
        />
        <USelectMenu
          v-model="eventLogStatus"
          :items="[
            { label: 'All Statuses', value: '' },
            { label: 'Success', value: 'success' },
            { label: 'Failed', value: 'failed' },
            { label: 'Partial', value: 'partial' },
          ]"
          class="w-36"
          size="xs"
          placeholder="All Statuses"
          value-key="value"
        />
      </div>

      <UCard>
        <div v-if="eventLogLoading" class="flex items-center justify-center py-8">
          <UIcon name="i-lucide-loader-2" class="size-6 animate-spin text-muted" />
        </div>
        <div v-else-if="eventLog && eventLog.events.length > 0">
          <div class="overflow-x-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="border-b border-default text-left text-muted">
                  <th class="pb-2 pr-3">Time</th>
                  <th class="pb-2 pr-3">Source</th>
                  <th class="pb-2 pr-3">Type</th>
                  <th class="pb-2 pr-3">Status</th>
                  <th class="pb-2 pr-3">Duration</th>
                  <th class="pb-2 pr-3">Items</th>
                  <th class="pb-2">Error</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="evt in eventLog.events"
                  :key="evt.id"
                  class="border-b border-default/30 cursor-pointer hover:bg-muted/10 transition-colors"
                  @click="selectedEvent = evt"
                >
                  <td class="py-1.5 pr-3 whitespace-nowrap text-muted">{{ timeAgo(evt.createdAt) }}</td>
                  <td class="py-1.5 pr-3 font-medium">{{ evt.sourceName }}</td>
                  <td class="py-1.5 pr-3">
                    <UBadge size="xs" variant="subtle" color="neutral">{{ evt.eventType }}</UBadge>
                  </td>
                  <td class="py-1.5 pr-3">
                    <UBadge size="xs" :color="statusColor(evt.status)">{{ evt.status }}</UBadge>
                  </td>
                  <td class="py-1.5 pr-3 font-mono">{{ formatDuration(evt.durationMs) }}</td>
                  <td class="py-1.5 pr-3">{{ evt.itemsCount ?? '-' }}</td>
                  <td class="py-1.5 max-w-xs truncate text-muted" :title="evt.errorMessage ?? ''">
                    {{ evt.errorMessage || '-' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Pagination -->
          <div class="mt-3 flex items-center justify-between text-xs text-muted">
            <span>{{ eventLog.total }} total events</span>
            <div class="flex gap-1">
              <UButton
                size="xs"
                variant="outline"
                icon="i-lucide-chevron-left"
                :disabled="eventLogPage === 0"
                @click="eventLogPage--"
              />
              <span class="flex items-center px-2">
                Page {{ eventLogPage + 1 }} of {{ Math.max(1, Math.ceil(eventLog.total / eventLogLimit)) }}
              </span>
              <UButton
                size="xs"
                variant="outline"
                icon="i-lucide-chevron-right"
                :disabled="(eventLogPage + 1) * eventLogLimit >= eventLog.total"
                @click="eventLogPage++"
              />
            </div>
          </div>
        </div>
        <div v-else class="flex items-center justify-center py-8 text-muted">
          <div class="text-center">
            <UIcon name="i-lucide-scroll-text" class="size-10 mx-auto mb-3 opacity-50" />
            <p class="text-sm">No events found</p>
          </div>
        </div>
      </UCard>
    </div>

    <!-- Event Detail Modal -->
    <UModal v-model:open="showEventModal" :ui="{ content: 'sm:max-w-2xl max-h-[85vh]' }">
      <template #header>
        <div class="flex items-center gap-2">
          <UIcon
            :name="selectedEvent?.status === 'failed' ? 'i-lucide-circle-x' : selectedEvent?.status === 'success' ? 'i-lucide-circle-check' : 'i-lucide-circle-alert'"
            :class="selectedEvent?.status === 'failed' ? 'text-red-500' : selectedEvent?.status === 'success' ? 'text-green-500' : 'text-yellow-500'"
            class="size-5"
          />
          <span class="font-semibold">Event Detail</span>
          <UBadge v-if="selectedEvent" size="xs" :color="statusColor(selectedEvent.status)">{{ selectedEvent.status }}</UBadge>
        </div>
      </template>

      <template #body>
        <div v-if="selectedEvent" class="space-y-4">
          <!-- Diagnosis Banner (for failed events) -->
          <div v-if="selectedEvent.status === 'failed' && diagnoseError(selectedEvent)" class="rounded-lg border border-red-500/20 bg-red-500/5 p-3">
            <div class="flex items-start gap-2">
              <UIcon name="i-lucide-stethoscope" class="size-4 text-red-400 mt-0.5 shrink-0" />
              <div>
                <div class="font-medium text-sm text-red-400">{{ diagnoseError(selectedEvent)!.title }}</div>
                <p class="text-xs text-muted mt-1">{{ diagnoseError(selectedEvent)!.explanation }}</p>
                <ul class="mt-2 space-y-1">
                  <li v-for="(tip, ti) in diagnoseError(selectedEvent)!.suggestions" :key="ti" class="flex items-start gap-1.5 text-xs text-muted">
                    <UIcon name="i-lucide-arrow-right" class="size-3 mt-0.5 shrink-0 text-red-400/60" />
                    <span>{{ tip }}</span>
                  </li>
                </ul>
              </div>
            </div>
          </div>

          <!-- Event Info Grid -->
          <div class="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
            <div>
              <div class="text-xs text-muted mb-0.5">Source</div>
              <div class="font-medium">{{ selectedEvent.sourceName }}</div>
            </div>
            <div>
              <div class="text-xs text-muted mb-0.5">Language</div>
              <div>{{ selectedEvent.language }}</div>
            </div>
            <div>
              <div class="text-xs text-muted mb-0.5">Operation</div>
              <UBadge size="xs" variant="subtle" color="neutral">{{ selectedEvent.eventType.replace(/_/g, ' ') }}</UBadge>
            </div>
            <div>
              <div class="text-xs text-muted mb-0.5">Duration</div>
              <div class="font-mono">{{ formatDuration(selectedEvent.durationMs) }}</div>
            </div>
            <div>
              <div class="text-xs text-muted mb-0.5">Timestamp</div>
              <div>{{ new Date(selectedEvent.createdAt).toLocaleString() }}</div>
            </div>
            <div>
              <div class="text-xs text-muted mb-0.5">Items Processed</div>
              <div>{{ selectedEvent.itemsCount ?? 'N/A' }}</div>
            </div>
            <div v-if="selectedEvent.errorCategory">
              <div class="text-xs text-muted mb-0.5">Error Category</div>
              <UBadge size="xs" :color="categoryColor(selectedEvent.errorCategory)">{{ selectedEvent.errorCategory }}</UBadge>
            </div>
            <div v-if="selectedEvent.metadata?.origin">
              <div class="text-xs text-muted mb-0.5">Triggered By</div>
              <div class="text-sm">{{ selectedEvent.metadata.origin === 'http_request' ? 'User action (HTTP)' : selectedEvent.metadata.origin === 'background_job' ? 'Background job' : selectedEvent.metadata.origin }}</div>
            </div>
          </div>

          <!-- Error Message -->
          <div v-if="selectedEvent.errorMessage">
            <div class="text-xs font-medium text-muted mb-1">Error Message</div>
            <div class="rounded-lg bg-red-500/5 border border-red-500/10 p-3 text-xs font-mono break-all text-red-300">
              {{ selectedEvent.errorMessage }}
            </div>
          </div>

          <!-- Metadata -->
          <div v-if="selectedEvent.metadata && Object.keys(selectedEvent.metadata).filter(k => k !== 'origin').length > 0">
            <div class="text-xs font-medium text-muted mb-1">Context</div>
            <div class="rounded-lg bg-muted/10 p-3 text-xs font-mono">
              <div v-for="(val, key) in selectedEvent.metadata" :key="key">
                <template v-if="key !== 'origin'">
                  <span class="text-muted">{{ key }}:</span> {{ val }}
                </template>
              </div>
            </div>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>
