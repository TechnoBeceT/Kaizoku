<script setup lang="ts">
definePageMeta({ layout: 'default' })
</script>

<template>
  <SettingsManager
    :show-header="true"
    :show-save-button="true"
    title="Settings"
    description="Configure your Kaizoku application settings"
  />
</template>
