<script setup lang="ts">
const props = defineProps<{
  error: { statusCode: number; message: string }
}>()

function handleError() {
  clearError({ redirect: '/' })
}
</script>

<template>
  <div class="flex items-center justify-center min-h-screen">
    <div class="text-center space-y-4">
      <h1 class="text-6xl font-bold text-muted">{{ error.statusCode }}</h1>
      <p class="text-lg text-muted">{{ error.message || 'An unexpected error occurred' }}</p>
      <UButton label="Go Home" icon="i-lucide-home" @click="handleError" />
    </div>
  </div>
</template>
