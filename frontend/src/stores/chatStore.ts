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
  
  // Computed endpoint that builds URL from config host and port
  const endpoint = computed(() => {
    // If config is available, construct endpoint from host and port
    if (configStore.config?.Host && configStore.config?.Port) {
      const protocol = configStore.config.Host === 'localhost' ? 'http' : 'https'
      return `${protocol}://${configStore.config.Host}:${configStore.config.Port}/api/v1/chat/completions`
    }
    // Fallback to hardcoded value if config not loaded yet
    return 'http://localhost:8080/api/v1/chat/completions'
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
