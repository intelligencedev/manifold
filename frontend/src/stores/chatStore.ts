import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
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
  const enableToolCalls = ref(false)
  const selectedSystemPrompt = ref('friendly_assistant')
  const system_prompt = ref('')
  const presence_penalty = ref()
  const top_p = ref()
  const top_k = ref()
  const min_p = ref()
  
  // Computed endpoint that builds URL from config based on provider
  const endpoint = computed(() => {
    let baseUrl = ''
    
    // First priority: use completions.default_host if available
    if (configStore.config?.Completions?.DefaultHost) {
      baseUrl = configStore.config.Completions.DefaultHost
    }
    // Second priority: construct from host and port if config is available  
    else if (configStore.config?.Host && configStore.config?.Port) {
      const protocol = configStore.config.Host === 'localhost' ? 'http' : 'https'
      baseUrl = `${protocol}://${configStore.config.Host}:${configStore.config.Port}/api/v1`
    }
    // Fallback to hardcoded base URL
    else {
      baseUrl = 'http://localhost:8080/api/v1'
    }
    
    // For providers that need /chat/completions appended
    if (provider.value === 'llama-server' || provider.value === 'mlx' || provider.value === 'react-agent') {
      // Check if baseUrl already ends with /chat/completions
      if (!baseUrl.endsWith('/chat/completions')) {
        return `${baseUrl}/chat/completions`
      }
    }
    // For OpenAI and other providers, use the base URL as-is (or with /chat/completions if needed)
    else if (provider.value === 'openai') {
      if (!baseUrl.endsWith('/chat/completions')) {
        return `${baseUrl}/chat/completions`
      }
    }
    
    return baseUrl
  })
  
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
    api_key,
    model,
    max_completion_tokens,
    temperature,
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
