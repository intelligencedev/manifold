import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useWorkflowStore = defineStore('workflow', () => {
  const isRunning = ref(false)
  const stopRequested = ref(false)
  const controller = ref<AbortController | null>(null)

  function start() {
    controller.value = new AbortController()
    stopRequested.value = false
    isRunning.value = true
  }

  function stop() {
    stopRequested.value = true
    if (controller.value) controller.value.abort()
    isRunning.value = false
  }

  const signal = computed(() => controller.value?.signal)

  return { isRunning, stopRequested, signal, start, stop }
})
