<template>
  <component :is="currentComponent" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useModeStore } from './stores/modeStore'
import App from './App.vue'
import Chat from './Chat.vue'

const modeStore = useModeStore()
const currentComponent = computed(() => modeStore.mode === 'chat' ? Chat : App)
</script>
