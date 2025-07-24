import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface ChatMessage {
  role: string;
  content: string;
}

export const useChatStore = defineStore('chat', () => {
  // Chat messages
  const messages = ref<ChatMessage[]>([])
  
  // Chat configuration
  const provider = ref('llama-server')
  const endpoint = ref('http://localhost:8080/api/v1/chat/completions')
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
    addMessage,
    updateLastAssistantMessage,
    clearMessages,
    setSystemPrompt,
    startGeneration,
    stopGeneration
  }
})
