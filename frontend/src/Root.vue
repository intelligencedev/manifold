<template>
  <Transition name="fade" mode="out-in" class="transition duration-100">
    <component :is="currentComponent" />
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useModeStore } from './stores/modeStore'
import App from './App.vue'
import Chat from './Chat.vue'

const modeStore = useModeStore()
const currentComponent = computed(() => modeStore.mode === 'chat' ? Chat : App)
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
