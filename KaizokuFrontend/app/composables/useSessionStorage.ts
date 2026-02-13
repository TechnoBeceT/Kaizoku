export function useSessionStorage<T>(key: string, defaultValue: T): Ref<T> {
  const prefixedKey = `kzk_${key}`

  const state = ref(defaultValue) as Ref<T>

  if (import.meta.client) {
    try {
      const stored = sessionStorage.getItem(prefixedKey)
      if (stored !== null) {
        state.value = JSON.parse(stored)
      }
    }
    catch {
      // Ignore parse errors, use default
    }
  }

  watch(state, (newVal) => {
    if (import.meta.client) {
      sessionStorage.setItem(prefixedKey, JSON.stringify(newVal))
    }
  }, { deep: true })

  return state
}
