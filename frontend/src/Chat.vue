<template>
  <div class="bg-zinc-800 text-gray-200 flex flex-col h-fit min-h-screen">
    <Header :mode="mode" @toggle-mode="toggleMode" />
    <div class="flex flex-1 overflow-hidden">
      <!-- Sidebar for parameters/settings -->
      <div class="bg-zinc-900 border-r border-zinc-700 w-80 min-w-[18rem] max-w-xs p-4 overflow-y-auto">
        <div class="space-y-2">
          <BaseDropdown label="Provider" v-model="provider" :options="providerOptions" />
          <BaseInput label="Endpoint" v-model="endpoint" />
          <BaseInput label="API Key" v-model="api_key" :type="showApiKey ? 'text' : 'password'">
            <template #suffix>
              <BaseTogglePassword v-model="showApiKey" />
            </template>
          </BaseInput>
          <BaseInput label="Model" v-model="model" />
          <div class="grid grid-cols-2 gap-2">
            <BaseInput label="Max Tokens" type="number" v-model.number="max_completion_tokens" min="1" />
            <BaseInput label="Temperature" type="number" v-model.number="temperature" step="0.1" min="0" max="2" />
          </div>
          <BaseCheckbox v-model="enableToolCalls" label="Enable Tool Calls" />
          <BaseDropdown label="Predefined System Prompt" v-model="selectedSystemPrompt" :options="systemPromptOptionsList" />
          <BaseTextarea label="System Prompt" v-model="system_prompt" />
          <BaseDropdown label="Render Mode" v-model="renderMode" :options="renderModeOptions" />
          <BaseDropdown v-if="renderMode === 'markdown'" label="Theme" v-model="selectedTheme" :options="themeOptions" />
        </div>
      </div>
      <!-- Chat/Main area -->
      <div class="flex-1 flex flex-col bg-zinc-800 overflow-hidden">
        <!-- messages -->
        <div ref="messageContainer" class="flex-1 overflow-y-auto space-y-4 p-4 pr-6 mb-12">
          <div v-for="(msg, i) in messages" :key="i" :class="msg.role === 'user' ? 'text-right' : 'text-left'">
            <div class="inline-block px-3 py-2 rounded max-w-lg" :class="msg.role==='user' ? 'bg-blue-600' : 'bg-zinc-700'">
              <div v-if="msg.role === 'assistant' && renderMode === 'markdown'" v-html="formatMessage(msg.content)" />
              <div v-else class="whitespace-pre-wrap">{{ msg.content }}</div>
            </div>
          </div>
        </div>
        <!-- input area - fixed at bottom -->
        <div class="mb-10 p-4 bg-zinc-800 border-t border-zinc-700">
          <BaseTextarea v-model="userInput" placeholder="Type a message..." class="w-full bg-zinc-700" />
          <div class="mt-2">
            <BaseButton @click="sendMessage">Send</BaseButton>
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
import BaseCheckbox from './components/base/BaseCheckbox.vue'
import BaseTogglePassword from './components/base/BaseTogglePassword.vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import { useModeStore } from './stores/modeStore'
import { useSystemPromptOptions } from './composables/systemPrompts'
import { useCompletionsApi } from './composables/useCompletionsApi'

const modeStore = useModeStore()
const mode = computed(() => modeStore.mode)
const toggleMode = () => modeStore.toggleMode()

const { systemPromptOptions, systemPromptOptionsList } = useSystemPromptOptions()
const { callCompletionsAPI } = useCompletionsApi()

// Use an explicit reactive array for messages to ensure Vue tracks changes properly
const messages = ref<{ role: string; content: string }[]>([])
const userInput = ref('')
const messageContainer = ref<HTMLElement | null>(null)
const showSettings = ref(false)
const showApiKey = ref(false)

const providerOptions = [
  { value: 'llama-server', label: 'llama-server' },
  { value: 'mlx_lm.server', label: 'mlx_lm.server' },
  { value: 'openai', label: 'openai' },
  { value: 'anthropic', label: 'anthropic' },
  { value: 'google', label: 'google' }
]

const provider = ref('llama-server')
const endpoint = ref('http://localhost:8080/api/v1/chat/completions')
const api_key = ref('')
const model = ref('local')
const max_completion_tokens = ref(8192)
const temperature = ref(0.6)
const enableToolCalls = ref(false)
const selectedSystemPrompt = ref('friendly_assistant')
const system_prompt = ref(systemPromptOptions[selectedSystemPrompt.value].system_prompt)
watch(selectedSystemPrompt, (k) => {
  if (systemPromptOptions[k]) system_prompt.value = systemPromptOptions[k].system_prompt
})

const renderModeOptions = [
  { value: 'raw', label: 'Raw Text' },
  { value: 'markdown', label: 'Markdown' }
]
const renderMode = ref('markdown')
const themeOptions = [
  { value: 'atom-one-dark', label: 'Dark' },
  { value: 'atom-one-light', label: 'Light' },
  { value: 'github', label: 'GitHub' },
  { value: 'monokai', label: 'Monokai' },
  { value: 'vs', label: 'VS' }
]
const selectedTheme = ref('atom-one-dark')
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
    }
  })
})

function toggleSettings() {
  showSettings.value = !showSettings.value
}

function formatMessage(content: string) {
  return marked(content)
}

async function sendMessage() {
  if (!userInput.value.trim()) return
  const prompt = userInput.value.trim()
  messages.value.push({ role: 'user', content: prompt })
  userInput.value = ''
  
  // Create the assistant message and add it to the messages array
  const msgIndex = messages.value.length
  messages.value.push({ role: 'assistant', content: '' })
  
  const config = {
    provider: provider.value,
    endpoint: endpoint.value,
    api_key: api_key.value,
    model: model.value,
    system_prompt: system_prompt.value,
    max_completion_tokens: max_completion_tokens.value,
    temperature: temperature.value,
    enableToolCalls: enableToolCalls.value
  }
  
  try {
    await callCompletionsAPI(config, prompt, (token: string) => {
      console.log('Received token:', token.substring(0, 50) + (token.length > 50 ? '...' : ''));
      // Create a new object to trigger reactivity properly
      const updatedMessages = [...messages.value]
      updatedMessages[msgIndex] = { 
        role: 'assistant', 
        content: updatedMessages[msgIndex].content + token 
      }
      messages.value = updatedMessages
    })
  } catch (e) {
    console.error(e)
    const updatedMessages = [...messages.value]
    updatedMessages[msgIndex] = { role: 'assistant', content: 'Error fetching response.' }
    messages.value = updatedMessages
  }
}
</script>
