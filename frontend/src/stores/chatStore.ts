import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { useConfigStore } from './configStore'

export interface ChatMessage {
  role: string;
  content: string;
}

export const useChatStore = defineStore('chat', () => {
  // Get config store to access backend configuration
  const configStore = useConfigStore()
  
  // Chat messages
  const messages = ref<ChatMessage[]>([])
  const thoughts = ref<string[]>([])
  
  // Chat configuration
  const provider = ref('llama-server')
  const api_key = ref('')
  const model = ref('local')
  const max_completion_tokens = ref(8192)
  const temperature = ref(0.6)
  const reasoning_effort = ref<'low' | 'medium' | 'high'>('low')
  const enableToolCalls = ref(false)
  const selectedSystemPrompt = ref('friendly_assistant')
  const system_prompt = ref('')
  const presence_penalty = ref()
  const top_p = ref()
  const top_k = ref()
  const min_p = ref()
  
  // Endpoint is now a writable ref, initialized from config but user-editable
  const endpoint = ref('')

  // Helper to compute default endpoint from config and provider
  function computeDefaultEndpoint() {
    let baseUrl = ''
    if (configStore.config?.Completions?.DefaultHost) {
      baseUrl = configStore.config.Completions.DefaultHost
    } else if (configStore.config?.Host && configStore.config?.Port) {
      const protocol = configStore.config.Host === 'localhost' ? 'http' : 'https'
      baseUrl = `${protocol}://${configStore.config.Host}:${configStore.config.Port}`
    } else {
      baseUrl = 'http://localhost:8080/api/v1'
    }
    return baseUrl
  }

  // Watch config and provider changes to update endpoint only if not manually overridden
  let endpointManuallySet = false
  function setEndpointManually(val: string) {
    endpoint.value = val
    endpointManuallySet = true
  }

  // Initialize endpoint on store creation
  endpoint.value = computeDefaultEndpoint()

  watch(
    [() => configStore.config, provider],
    () => {
      if (!endpointManuallySet) {
        endpoint.value = computeDefaultEndpoint()
      }
    },
    { immediate: true, deep: true }
  )

  // Optionally, expose a method to reset endpoint to default
  function resetEndpointToDefault() {
    endpoint.value = computeDefaultEndpoint()
    endpointManuallySet = false
  }
  
  // UI preferences
  const renderMode = ref('markdown')
  const selectedTheme = ref('atom-one-dark')

  // Generation state
  const isGenerating = ref(false)
  const stopRequested = ref(false)
  const controller = ref<AbortController | null>(null)

  function startGeneration() {
    controller.value = new AbortController()
    stopRequested.value = false
    isGenerating.value = true
  }

  function stopGeneration() {
    stopRequested.value = true
    if (controller.value) controller.value.abort()
    isGenerating.value = false
  }

  const signal = computed(() => controller.value?.signal)

  // Methods to update chat state
  function addMessage(message: ChatMessage) {
    messages.value.push(message)
  }

  function addThought(thought: string) {
    thoughts.value.push(thought)
  }

  function clearThoughts() {
    thoughts.value = []
  }
  
  function updateLastAssistantMessage(content: string) {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].role === 'assistant') {
        messages.value[i].content = content
        break
      }
    }
  }
  
  function clearMessages() {
    messages.value = []
  }
  
  function setSystemPrompt(prompt: string) {
    system_prompt.value = prompt
  }

  return {
    messages, 
    provider,
  endpoint,
  setEndpointManually,
  resetEndpointToDefault,
    api_key,
    model,
    max_completion_tokens,
    temperature,
    reasoning_effort,
    enableToolCalls,
    selectedSystemPrompt,
    system_prompt,
    renderMode,
    selectedTheme,
    presence_penalty,
    top_p,
    top_k,
    min_p,
    isGenerating,
    stopRequested,
    signal,
    thoughts,
    addThought,
    clearThoughts,
    addMessage,
    updateLastAssistantMessage,
    clearMessages,
    setSystemPrompt,
    startGeneration,
    stopGeneration
  }
})
