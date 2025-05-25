import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useModeStore = defineStore('mode', () => {
  const mode = ref<'flow' | 'chat'>('flow')
  function toggleMode () {
    mode.value = mode.value === 'flow' ? 'chat' : 'flow'
  }
  function setMode (m: 'flow' | 'chat') {
    mode.value = m
  }
  return { mode, toggleMode, setMode }
})
