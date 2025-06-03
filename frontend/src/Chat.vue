<template>
  <div class="bg-zinc-800 text-gray-200 flex flex-col h-screen view-container font-roboto antialiased">
    <Header :mode="mode" @toggle-mode="toggleMode" />
    <div class="flex flex-1 overflow-hidden">
      <!-- Sidebar for parameters/settings -->
      <div class="bg-zinc-900 border-r border-zinc-700 w-80 min-w-[18rem] max-w-xs p-4 overflow-y-auto sidebar-scroll">
        <div class="space-y-6">
          <BaseDropdown label="Provider" v-model="provider" :options="providerOptions" />
          <BaseInput label="Endpoint" v-model="endpoint" @blur="fetchLocalServerModel" />
          <BaseInput label="API Key" v-model="api_key" :type="showApiKey ? 'text' : 'password'">
            <template #suffix>
              <BaseTogglePassword v-model="showApiKey" />
            </template>
          </BaseInput>
          <div class="relative">
            <BaseInput label="Model" v-model="model" :disabled="isLoadingModel" />
            <span v-if="isLoadingModel" class="absolute right-10 top-1/2 transform -translate-y-1/2 text-xs text-blue-400">Loading...</span>
          </div>
          <div class="grid grid-cols-2 gap-2 space-y-6">
            <BaseInput label="Max Tokens" type="number" v-model.number="max_completion_tokens" min="1" />
            <BaseInput label="Temperature" type="number" v-model.number="temperature" step="0.1" min="0" max="2" />
          </div>
          <!-- Extra LLM params for openai, llama-server, mlx_lm.server -->
          <template v-if="['openai','llama-server','mlx_lm.server'].includes(provider)">
            <div class="grid grid-cols-2 gap-2 mt-2">
              <BaseInput label="Presence Penalty" type="number" v-model.number="presence_penalty" step="0.01" min="-2" max="2" />
              <BaseInput label="Top P" type="number" v-model.number="top_p" step="0.01" min="0" max="1" />
              <BaseInput label="Top K" type="number" v-model.number="top_k" min="0" :disabled="provider !== 'mlx_lm.server'" />
              <BaseInput label="Min P" type="number" v-model.number="min_p" step="0.01" min="0" max="1" />
            </div>
          </template>
          <BaseDropdown label="Predefined System Prompt" v-model="selectedSystemPrompt" :options="systemPromptOptionsList" />
          <BaseTextarea label="System Prompt" v-model="system_prompt" />
          <BaseDropdown label="Render Mode" v-model="renderMode" :options="renderModeOptions" />
        </div>
      </div>
      <!-- Chat/Main area -->
      <div class="flex-1 flex flex-col bg-zinc-800 overflow-hidden">
        <!-- messages -->
        <div ref="messageContainer" class="message-area-scroll flex-1 overflow-y-auto space-y-6 p-4 2xl:px-65 xl:px-45">
          <div v-for="(msg, i) in messages" :key="i" :class="msg.role === 'user' ? 'text-right' : ''">
            <div class="p-6 rounded-lg" :class="msg.role==='user' ? 'bg-teal-600 inline-block px-3 py-2 w-1/2 text-left' : ''">
              <div v-if="msg.role === 'assistant' && renderMode === 'markdown'" class="markdown-content" v-html="formatMessage(msg.content)" />
              <div v-else class="whitespace-pre-wrap">{{ msg.content }}</div>
            </div>
          </div>
        </div>
        <!-- input area - fixed at bottom -->
        <div class="mb-10 px-4 bg-zinc-800 2xl:px-65 xl:px-45">
            <BaseTextarea v-model="userInput" placeholder="Type a message..." class="w-full bg-zinc-700 border-teal-700 border-1 rounded-lg" />
          <div class="mt-2 px-6">
            <BaseButton class="bg-teal-700 hover:bg-teal-600 w-full" @click="sendMessage">Send</BaseButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, computed } from 'vue'
import Header from './components/layout/Header.vue'
import BaseButton from './components/base/BaseButton.vue'
import BaseInput from './components/base/BaseInput.vue'
import BaseTextarea from './components/base/BaseTextarea.vue'
import BaseDropdown from './components/base/BaseDropdown.vue'
import BaseTogglePassword from './components/base/BaseTogglePassword.vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import { useModeStore } from './stores/modeStore'
import { useChatStore } from './stores/chatStore'
import { useSystemPromptOptions } from './composables/systemPrompts'
import { useCompletionsApi } from './composables/useCompletionsApi'
import { useConfigStore } from './stores/configStore'

// Add type declaration for marked options
declare module 'marked' {
  interface MarkedOptions {
    highlight?: (code: string, lang: string) => string;
  }
}

const modeStore = useModeStore()
const chatStore = useChatStore()
const mode = computed(() => modeStore.mode)
const toggleMode = () => modeStore.toggleMode()

const { systemPromptOptions, systemPromptOptionsList } = useSystemPromptOptions()
const { callCompletionsAPI } = useCompletionsApi()
const configStore = useConfigStore()
const agentMaxSteps = computed(() => configStore.config?.Completions?.Agent?.MaxSteps || 30)

// Use the messages from chatStore instead of local ref
const messages = computed(() => chatStore.messages)
const userInput = ref('')
const messageContainer = ref<HTMLElement | null>(null)
const showApiKey = ref(false)
const isLoadingModel = ref(false) // Track model loading state

const providerOptions = [
  { value: 'llama-server', label: 'llama-server' },
  { value: 'mlx_lm.server', label: 'mlx_lm.server' },
  { value: 'openai', label: 'openai' },
  { value: 'anthropic', label: 'anthropic' },
  { value: 'google', label: 'google' },
  { value: 'react-agent', label: 'ReAct Agent' }
]

// Use values from chatStore
const provider = computed({
  get: () => chatStore.provider,
  set: (value) => chatStore.provider = value
})

const endpoint = computed({
  get: () => chatStore.endpoint,
  set: (value) => chatStore.endpoint = value
})

const api_key = computed({
  get: () => chatStore.api_key,
  set: (value) => chatStore.api_key = value
})

watch(
  () => configStore.config,
  (newConfig) => {
    if (newConfig && newConfig.Completions) {
      if (!chatStore.api_key && newConfig.Completions.APIKey) {
        chatStore.api_key = newConfig.Completions.APIKey
      }
      if (!chatStore.endpoint && newConfig.Completions.DefaultHost) {
        chatStore.endpoint = newConfig.Completions.DefaultHost
      }
    }
  },
  { immediate: true, deep: true }
)

watch(
  () => configStore.config?.Completions?.Provider,
  (newProvider) => {
    if (newProvider && provider.value !== 'openai') {
      chatStore.endpoint = configStore.config.Completions.DefaultHost
    }
  },
  { immediate: true }
)

const model = computed({
  get: () => chatStore.model,
  set: (value) => chatStore.model = value
})

const max_completion_tokens = computed({
  get: () => chatStore.max_completion_tokens,
  set: (value) => chatStore.max_completion_tokens = value
})

const temperature = computed({
  get: () => chatStore.temperature,
  set: (value) => chatStore.temperature = value
})

const presence_penalty = computed({
  get: () => chatStore.presence_penalty,
  set: (value) => chatStore.presence_penalty = value
})
const top_p = computed({
  get: () => chatStore.top_p,
  set: (value) => chatStore.top_p = value
})
const top_k = computed({
  get: () => chatStore.top_k,
  set: (value) => chatStore.top_k = value
})
const min_p = computed({
  get: () => chatStore.min_p,
  set: (value) => chatStore.min_p = value
})

const selectedSystemPrompt = computed({
  get: () => chatStore.selectedSystemPrompt,
  set: (value) => chatStore.selectedSystemPrompt = value
})

const system_prompt = computed({
  get: () => chatStore.system_prompt,
  set: (value) => chatStore.system_prompt = value
})

// Initialize system prompt if not already set
if (!chatStore.system_prompt && selectedSystemPrompt.value in systemPromptOptions) {
  const key = selectedSystemPrompt.value as keyof typeof systemPromptOptions
  chatStore.setSystemPrompt(systemPromptOptions[key].system_prompt)
}

watch(selectedSystemPrompt, (k) => {
  const key = k as keyof typeof systemPromptOptions
  if (key in systemPromptOptions) {
    chatStore.setSystemPrompt(systemPromptOptions[key].system_prompt)
  }
})

// Watch provider changes to fetch model from llama-server or mlx_lm.server
watch(provider, (newProvider) => {
  if ((newProvider === 'llama-server' || newProvider === 'mlx_lm.server') && endpoint.value) {
    fetchLocalServerModel()
  }
})

// Helper function to fetch model ID from local servers (llama-server or mlx_lm.server)
async function fetchLocalServerModel() {
  if ((provider.value !== 'llama-server' && provider.value !== 'mlx_lm.server') || !endpoint.value) return;
  isLoadingModel.value = true
  try {
    // Derive the models endpoint from the chat completions endpoint
    let modelsEndpoint = endpoint.value
    // Extract the base URL from the endpoint
    const endpointParts = modelsEndpoint.split('/')
    const apiIndex = endpointParts.findIndex(part => part === 'api' || part === 'v1')
    let baseUrl
    if (apiIndex !== -1) {
      baseUrl = endpointParts.slice(0, apiIndex).join('/')
      modelsEndpoint = `${baseUrl}/v1/models`
    } else {
      const urlObj = new URL(modelsEndpoint)
      urlObj.pathname = '/v1/models'
      modelsEndpoint = urlObj.toString()
    }
    console.log("Fetching model from:", modelsEndpoint)
    const response = await fetch(modelsEndpoint)
    if (!response.ok) {
      throw new Error(`Failed to fetch models: ${response.statusText}`)
    }
    const data = await response.json()
    if (data && data.data && data.data.length > 0) {
      const modelId = data.data[0].id
      console.log("Found model:", modelId)
      chatStore.model = modelId
    }
  } catch (error) {
    console.error("Error fetching local server model:", error)
  } finally {
    isLoadingModel.value = false
  }
}

const renderModeOptions = [
  { value: 'raw', label: 'Raw Text' },
  { value: 'markdown', label: 'Markdown' }
]

const renderMode = computed({
  get: () => chatStore.renderMode,
  set: (value) => chatStore.renderMode = value
})

// Theme options are used through the selectedTheme computed property

const selectedTheme = computed({
  get: () => chatStore.selectedTheme,
  set: (value) => chatStore.selectedTheme = value
})

let currentThemeLink: HTMLLinkElement | null = null
function loadTheme(themeName: string) {
  if (currentThemeLink) document.head.removeChild(currentThemeLink)
  currentThemeLink = document.createElement('link')
  currentThemeLink.rel = 'stylesheet'
  currentThemeLink.href = `https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.7.0/styles/${themeName}.min.css`
  document.head.appendChild(currentThemeLink)
}
onMounted(() => {
  loadTheme(selectedTheme.value)
  marked.setOptions({
    highlight(code, lang) {
      if (lang && hljs.getLanguage(lang)) {
        try { return hljs.highlight(code, { language: lang }).value } catch {}
      }
      try { return hljs.highlightAuto(code).value } catch {}
      return code
    }
  })
})
watch(selectedTheme, loadTheme)
watch(messages, () => {
  nextTick(() => {
    if (messageContainer.value) {
      messageContainer.value.scrollTop = messageContainer.value.scrollHeight
      
      // Highlight code blocks in messages
      const codeBlocks = messageContainer.value.querySelectorAll('pre code:not(.hljs)')
      codeBlocks.forEach(block => {
        hljs.highlightElement(block as HTMLElement)
      })
    }
  })
}, { deep: true })

// Removed the old fetchLlamaServerModel function

function createEventStreamSplitter () {
  let buffer = ''
  return new TransformStream<string, string>({
    transform (chunk, controller) {
      buffer += chunk
      let idx
      while ((idx = buffer.indexOf('\n\n')) !== -1) {
        const event = buffer.slice(0, idx).replace(/^data:\s*/gm, '').trim()
        controller.enqueue(event)
        buffer = buffer.slice(idx + 2)
      }
    },
    flush (controller) {
      if (buffer.trim()) {
        const event = buffer.replace(/^data:\s*/gm, '').trim()
        if (event) {
          controller.enqueue(event)
        }
      }
    }
  })
}

function formatMessage(content: string) {
  return marked(content)
}

async function sendMessage() {
  if (!userInput.value.trim()) return
  const prompt = userInput.value.trim()
  chatStore.addMessage({ role: 'user', content: prompt })
  userInput.value = ''

  // Create the assistant message and add it to the messages array
  chatStore.addMessage({ role: 'assistant', content: '' })

  if (provider.value === 'react-agent') {
    try {
      const resp = await fetch('http://localhost:8080/api/agents/react/stream', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'text/event-stream'
        },
        body: JSON.stringify({
          objective: prompt,
          max_steps: agentMaxSteps.value,
          model: ''
        })
      })

      if (!resp.ok || !resp.body) {
        throw new Error(`API error (${resp.status})`)
      }

      const reader = resp.body
        .pipeThrough(new TextDecoderStream())
        .pipeThrough(createEventStreamSplitter())
        .getReader()

      let thoughts = ''
      let finalResult = ''
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        if (value === '[[EOF]]') {
          await reader.cancel()
          break
        }
        const thinkMatch = value.match(/<think>([\s\S]*?)<\/think>/)
        if (thinkMatch) {
          thoughts += thinkMatch[1] + '\n'
        } else {
          finalResult = value
        }
        const combined = (thoughts ? `<think>${thoughts}</think>` : '') + (finalResult ? `\n${finalResult}` : '')
        chatStore.updateLastAssistantMessage(combined)
      }
      const finalResponse = (thoughts ? `<think>${thoughts}</think>` : '') + (finalResult ? `\n${finalResult}` : '')
      chatStore.updateLastAssistantMessage(finalResponse)
    } catch (e) {
      console.error(e)
      chatStore.updateLastAssistantMessage('Error fetching response.')
    }
    return
  }

  const config: Record<string, any> = {
    provider: provider.value,
    endpoint: endpoint.value,
    api_key: api_key.value,
    model: model.value,
    system_prompt: system_prompt.value,
    max_completion_tokens: max_completion_tokens.value,
    temperature: temperature.value,
  }
  if (["openai", "llama-server", "mlx_lm.server"].includes(provider.value)) {
    if (presence_penalty.value !== undefined && presence_penalty.value !== null && presence_penalty.value !== '') config.presence_penalty = presence_penalty.value
    if (top_p.value !== undefined && top_p.value !== null && top_p.value !== '') config.top_p = top_p.value
    if (min_p.value !== undefined && min_p.value !== null && min_p.value !== '') config.min_p = min_p.value
    // Only include top_k for mlx_lm.server
    if (provider.value === 'mlx_lm.server' && top_k.value !== undefined && top_k.value !== null && top_k.value !== '') config.top_k = top_k.value
  }

  try {
    let assistantResponse = ''

    await callCompletionsAPI(config, prompt, (token: string) => {
      console.log('Received token:', token.substring(0, 50) + (token.length > 50 ? '...' : ''))
      assistantResponse += token
      // Update the last assistant message in the chatStore
      chatStore.updateLastAssistantMessage(assistantResponse)
    })
  } catch (e) {
    console.error(e)
    chatStore.updateLastAssistantMessage('Error fetching response.')
  }
}
</script>

<style>
/* Styling for code blocks similar to ResponseNode */
.markdown-content pre {
  background: rgba(45, 45, 55, 0.6);
  padding: 12px;
  border-radius: 6px;
  margin: 12px 0;
  overflow-x: auto;
  border-left: 3px solid #8a70b5;
}

.markdown-content code {
  font-family: 'Fira Code', 'Courier New', Courier, monospace;
  font-size: 14px;
}

.markdown-content code:not(pre code) {
  background: rgba(73, 49, 99, 0.3);
  padding: 2px 5px;
  border-radius: 4px;
}

.markdown-content pre code {
  background: transparent;
  padding: 0;
  border-radius: 0;
  color: #e1e1e6;
}

/* Scrollbar styling for sidebar */
.sidebar-scroll {
  scrollbar-width: thin;
  scrollbar-color: oklch(60% 0.118 184.704) transparent; /* teal-500 thumb, no track */
}

.sidebar-scroll::-webkit-scrollbar {
  width: 6px;
}

.sidebar-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-scroll::-webkit-scrollbar-thumb {
  background-color: oklch(60% 0.118 184.704);
  border-radius: 9999px;
}

/* Scrollbar styling for message area */
.message-area-scroll {
  scrollbar-width: thin;
  scrollbar-color: oklch(60% 0.118 184.704) transparent; /* teal-500 thumb, no track */
}

.message-area-scroll::-webkit-scrollbar {
  width: 6px;
}

.message-area-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.message-area-scroll::-webkit-scrollbar-thumb {
  background-color: oklch(60% 0.118 184.704);
  border-radius: 9999px;
}
</style>
