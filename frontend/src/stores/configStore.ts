import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useConfigStore = defineStore('config', () => {
  const config = ref<any>(null)
  const loading = ref<boolean>(false)
  const error = ref<string | null>(null)

  // Fetch the configuration from the backend
  const fetchConfig = async () => {
    loading.value = true
    try {
      const response = await axios.get('/api/config')
      config.value = response.data
      console.log('Fetched config:', config.value) // Log the config to the console
    } catch (err) {
      error.value = 'Failed to load config'
    } finally {
      loading.value = false
    }
  }

  return { config, loading, error, fetchConfig }
})
