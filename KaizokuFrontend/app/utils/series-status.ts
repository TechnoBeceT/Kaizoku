import { SeriesStatus } from '~/types'

export interface StatusDisplay {
  text: string
  color: string
}

export function getStatusDisplay(status: SeriesStatus): StatusDisplay {
  switch (status) {
    case SeriesStatus.ONGOING:
      return { text: 'Ongoing', color: 'success' }
    case SeriesStatus.COMPLETED:
      return { text: 'Completed', color: 'info' }
    case SeriesStatus.LICENSED:
      return { text: 'Licensed', color: 'primary' }
    case SeriesStatus.PUBLISHING_FINISHED:
      return { text: 'Publishing Finished', color: 'info' }
    case SeriesStatus.CANCELLED:
      return { text: 'Cancelled', color: 'error' }
    case SeriesStatus.ON_HIATUS:
      return { text: 'On Hiatus', color: 'warning' }
    case SeriesStatus.DISABLED:
      return { text: 'Disabled', color: 'neutral' }
    default:
      return { text: 'Unknown', color: 'neutral' }
  }
}
