export function useDebounce<T>(value: Ref<T>, delay: number): Readonly<Ref<T>> {
  const debounced = ref(value.value) as Ref<T>
  let timeout: ReturnType<typeof setTimeout>

  watch(value, (newVal) => {
    clearTimeout(timeout)
    timeout = setTimeout(() => {
      debounced.value = newVal
    }, delay)
  })

  onUnmounted(() => {
    clearTimeout(timeout)
  })

  return readonly(debounced)
}
