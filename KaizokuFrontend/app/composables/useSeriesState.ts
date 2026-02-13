export function useSeriesState() {
  const seriesTitle = useState('kzk-series-title', () => '')

  function setSeriesTitle(title: string) {
    seriesTitle.value = title
  }

  return {
    seriesTitle: readonly(seriesTitle),
    setSeriesTitle,
  }
}
