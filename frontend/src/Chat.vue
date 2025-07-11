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
          <template v-if="['openai','llama-server','mlx_lm.server'].includes(provider)">
            <div class="grid grid-cols-2 gap-2 mt-2">
              <BaseInput label="Presence Penalty" type="number" v-model.number="presence_penalty" step="0.01" min="-2" max="2" />
          <template v-if="['openai','llama-server','mlx_lm.server'].includes(provider)">
            <div class="grid grid-cols-2 gap-2 mt-2">
              <BaseInput label="Presence Penalty" type="number" v-model.number="presence_penalty" step="0.01" min="-2" max="2" />
              <BaseInput label="Top P" type="number" v-model.number="top_p" step="0.01" min="0" max="1" />
              <BaseInput label="Top K" type="number" v-model.number="top_k" min="0" />
              <BaseInput label="Min P" type="number" v-model.number="min_p" step="0.01" min="0" max="1" />
            </div>
            <!-- Debug info -->
            <div class="text-xs text-gray-400 mt-2">
              Debug: Provider = {{ provider }}
            </div>
          </template>
      <div class="flex-1 flex flex-col bg-zinc-800 overflow-hidden">
        <!-- messages -->
        <div ref="messageContainer" class="w-full message-area-scroll flex-1 overflow-y-auto space-y-6 p-4 xl:px-65">
          <div v-for="(msg, i) in messages" :key="i" :class="msg.role === 'user' ? 'text-right' : ''">
            <div class="p-6 rounded-lg" :class="msg.role==='user' ? 'bg-teal-600 inline-block px-3 py-2 w-1/2 text-left' : ''">
              <div v-if="msg.role === 'assistant' && renderMode === 'markdown'" class="markdown-content" v-html="formatMessage(msg.content)" />
              <div v-else class="whitespace-pre-wrap">{{ msg.content }}</div>
            </div>
          </div>
        </div>
        <!-- input area - fixed at bottom -->
        <div class="relative flex w-full items-end mx-4 px-4 pb-4 xl:px-65 bg-zinc-800">
          <div class="relative flex w-full flex-auto flex-col">
            <!-- Main input container with modern styling -->
            <div class="relative mx-2.5 flex w-full">
              <div class="relative flex w-full flex-auto bg-zinc-700 rounded-xl border border-zinc-600 transition-colors">
                <!-- Textarea container -->
                <div class="flex-1 relative">
                  <textarea
                    ref="textareaRef"
                    v-model="userInput"
                    placeholder="Type a message..."
                    rows="1"
                    class="block w-full resize-none bg-transparent rounded-xl p-4 pr-20 my-6 text-gray-200 placeholder-gray-400 border-0 min-h-12 no-focus-anywhere"
                    style="max-height: 240px; overflow-y: auto;"
                    @input="autoResize"
                    @keydown="handleTextareaKeydown"
                  />
                </div>
                <!-- Send button positioned inside the input -->
                <div class="absolute right-2 bottom-1 flex items-center">
                  <BaseButton
                  @click="sendMessage"
                  class="mr-4 mb-4 flex items-center justify-center rounded-lg transition-colors hover:opacity-70 disabled:opacity-50 bg-teal-600 hover:bg-teal-700 text-white h-10 w-10 focus:outline-none focus:ring-2 focus:ring-teal-500 focus:ring-offset-2 focus:ring-offset-zinc-700"
                  :disabled="!userInput.trim()"
                  >
                  <span class="sr-only">Send</span>
                  <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24"
                     class="h-9 w-9">
                    <!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ -->
                    <path fill="currentColor" fill-rule="evenodd"
                      d="M17.53 10.03a.75.75 0 0 0 0-1.06l-5-5a.75.75 0 0 0-1.06 0l-5 5a.75.75 0 1 0 1.06 1.06l3.72-3.72v8.19c0 .713-.22 1.8-.859 2.687c-.61.848-1.635 1.563-3.391 1.563a.75.75 0 0 0 0 1.5c2.244 0 3.72-.952 4.609-2.187c.861-1.196 1.141-2.61 1.141-3.563V6.31l3.72 3.72a.75.75 0 0 0 1.06 0"
                      clip-rule="evenodd" />
                  </svg>
                  </BaseButton>
                </div>
              </div>
            </div>
            <!-- Spacer to maintain layout -->
            <div style="height: 12px;"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
function handleTextareaKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === 'Return') {
    if (e.shiftKey) {
      // Allow newline
      return
    } else {
      e.preventDefault()
      sendMessage()
    }
  }
}

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
const textareaRef = ref<HTMLTextAreaElement | null>(null)
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
      // Add copy buttons after highlighting
      addCopyButtons()
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

// Add copy buttons to code blocks
function addCopyButtons() {
  if (!messageContainer.value) return
  const pres = messageContainer.value.querySelectorAll('pre')
  pres.forEach(pre => {
    // avoid duplicate
    if (pre.querySelector('.copy-btn')) return
    // ensure relative positioning
    (pre as HTMLElement).style.position = 'relative'
    const btn = document.createElement('button')
    btn.innerText = 'Copy'
    btn.className = 'copy-btn absolute top-2 right-2 bg-zinc-700 hover:bg-zinc-600 text-xs text-gray-200 px-2 py-1 rounded'
    btn.onclick = async () => {
      try {
        await navigator.clipboard.writeText((pre.querySelector('code')?.textContent) || '')
        btn.innerText = 'Copied'
        setTimeout(() => { btn.innerText = 'Copy' }, 2000)
      } catch (e) {
        console.error('Copy failed', e)
      }
    }
    pre.appendChild(btn)
  })
}

// Watch messages for new code blocks
 watch(messages, () => {
   nextTick(() => {
     if (messageContainer.value) {
       messageContainer.value.scrollTop = messageContainer.value.scrollHeight
       // Highlight code blocks in messages
       const codeBlocks = messageContainer.value.querySelectorAll('pre code:not(.hljs)')
       codeBlocks.forEach(block => {
         hljs.highlightElement(block as HTMLElement)
       })
      // Add copy buttons after highlighting
      addCopyButtons()
     }
   })
 }, { deep: true })

async function sendMessage() {
  if (!userInput.value.trim()) return
  const prompt = userInput.value.trim()
  chatStore.addMessage({ role: 'user', content: prompt })
  userInput.value = ''
  
  // Reset textarea height after clearing input
  resetTextareaHeight()

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

// Auto-resize textarea function
function autoResize() {
  if (textareaRef.value) {
    textareaRef.value.style.height = 'auto'
    const scrollHeight = textareaRef.value.scrollHeight
    const maxHeight = 240 // 10 lines * 24px line-height
    textareaRef.value.style.height = Math.min(scrollHeight, maxHeight) + 'px'
  }
}

// Reset textarea to original height
function resetTextareaHeight() {
  if (textareaRef.value) {
    textareaRef.value.style.height = ''
    textareaRef.value.style.removeProperty('height')
  }
}
</script>

<style>
/* NUCLEAR OPTION - Remove ALL possible focus styles */
.no-focus-anywhere,
.no-focus-anywhere:focus,
.no-focus-anywhere:focus-visible,
.no-focus-anywhere:focus-within,
.no-focus-anywhere:active,
.no-focus-anywhere:hover {
  outline: none !important;
  box-shadow: none !important;
  border: 0 !important;
  border-color: transparent !important;
  border-width: 0 !important;
  --tw-ring-shadow: none !important;
  --tw-ring-offset-shadow: none !important;
}

/* Target all possible textarea states */
textarea.no-focus-anywhere,
textarea.no-focus-anywhere:focus,
textarea.no-focus-anywhere:focus-visible,
textarea.no-focus-anywhere:focus-within,
textarea.no-focus-anywhere:active,
textarea.no-focus-anywhere:hover {
  outline: none !important;
  box-shadow: none !important;
  border: 0 !important;
  border-color: transparent !important;
  border-width: 0 !important;
  --tw-ring-shadow: none !important;
  --tw-ring-offset-shadow: none !important;
}

/* Override any Tailwind focus utilities */
.no-focus-anywhere:focus {
  --tw-ring-offset-shadow: var(--tw-ring-inset) 0 0 0 var(--tw-ring-offset-width) var(--tw-ring-offset-color) !important;
  --tw-ring-shadow: var(--tw-ring-inset) 0 0 0 calc(0px + var(--tw-ring-offset-width)) var(--tw-ring-color) !important;
  box-shadow: var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow, 0 0 #0000) !important;
}

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

.markdown-content {
  line-height: 1.8;
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

/* Scrollbar styling for textarea */
.no-focus-anywhere {
  scrollbar-width: none !important; /* Firefox */
  -ms-overflow-style: none !important; /* IE and Edge */
}

.no-focus-anywhere::-webkit-scrollbar {
  display: none !important; /* Chrome, Safari, Opera */
}

.no-focus-anywhere::-webkit-scrollbar-track {
  display: none !important;
}

.no-focus-anywhere::-webkit-scrollbar-thumb {
  display: none !important;
}
</style>
