<template>
  <div class="grid grid-rows-[1fr_auto] h-[75vh] max-h-[75vh] rounded-xl border border-slate-800 bg-slate-900/50">
    <div class="overflow-y-auto p-4 space-y-4" ref="messagesEl">
      <div v-for="(m, i) in messages" :key="i" class="flex" :class="m.role === 'user' ? 'justify-end' : 'justify-start'">
        <div class="max-w-[75%] rounded-lg px-4 py-2 text-sm"
             :class="m.role === 'user' ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-100'">
          <p class="whitespace-pre-wrap">{{ m.content }}</p>
        </div>
      </div>
    </div>
    <form class="border-t border-slate-800 p-3 flex gap-2" @submit.prevent="send">
      <input v-model="draft" type="text" placeholder="Message the agent..."
             class="flex-1 rounded-lg bg-slate-800/70 border border-slate-700 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-emerald-500" />
      <button type="submit" :disabled="!draft.trim()"
              class="rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 disabled:cursor-not-allowed px-4 py-2 text-sm font-semibold text-white">
        Send
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick } from 'vue'

type Msg = { role: 'user' | 'assistant'; content: string }

const messages = ref<Msg[]>([
  { role: 'assistant', content: 'Hi! I\'m your agent. How can I help?' }
])
const draft = ref('')
const messagesEl = ref<HTMLDivElement | null>(null)

function scrollToBottom() {
  nextTick(() => {
    messagesEl.value?.scrollTo({ top: messagesEl.value.scrollHeight, behavior: 'smooth' })
  })
}

function send() {
  const text = draft.value.trim()
  if (!text) return
  messages.value.push({ role: 'user', content: text })
  draft.value = ''
  scrollToBottom()
  // Simulate assistant reply for now
  setTimeout(() => {
    messages.value.push({ role: 'assistant', content: `You said: ${text}` })
    scrollToBottom()
  }, 500)
}
</script>
