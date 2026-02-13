type PageType = 'library' | 'providers' | 'queue' | 'settings' | 'series' | 'cloud-latest' | 'other'

export function useSearchState() {
  const searchTerm = useState('kzk-search', () => {
    if (import.meta.server) return ''
    return sessionStorage.getItem('kzk_search') || ''
  })

  const route = useRoute()

  const debouncedSearchTerm = useDebounce(searchTerm, 300)

  const currentPage = computed<PageType>(() => {
    const path = route.path
    if (path === '/' || path === '/library') return 'library'
    if (path === '/providers') return 'providers'
    if (path === '/queue') return 'queue'
    if (path === '/settings') return 'settings'
    if (path === '/cloud-latest') return 'cloud-latest'
    if (path.startsWith('/library/series')) return 'series'
    return 'other'
  })

  const isSearchDisabled = computed(() => {
    return currentPage.value === 'settings' || currentPage.value === 'series'
  })

  function setSearchTerm(term: string) {
    searchTerm.value = term
    if (import.meta.client) {
      sessionStorage.setItem('kzk_search', term)
    }
  }

  function clearSearch() {
    searchTerm.value = ''
    if (import.meta.client) {
      sessionStorage.setItem('kzk_search', '')
    }
  }

  return {
    searchTerm: readonly(searchTerm),
    debouncedSearchTerm,
    setSearchTerm,
    clearSearch,
    currentPage,
    isSearchDisabled,
  }
}
