<template>
  <section class="space-y-6">
    <header class="space-y-2">
      <h1 class="text-2xl font-semibold text-white">Settings</h1>
      <p class="text-sm text-slate-400">
        Configure integrations, authentication, and advanced execution knobs for agentd.
      </p>
    </header>

    <div class="space-y-6">
      <form class="space-y-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
        <div>
          <label class="text-sm font-medium text-slate-300" for="api-url">API Base URL</label>
          <p class="text-xs text-slate-500">Used during local development when the Go backend is proxied.</p>
          <input
            id="api-url"
            v-model="apiUrl"
            type="url"
            placeholder="https://localhost:32180/api"
            class="mt-2 w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 focus:border-emerald-400 focus:outline-none focus:ring focus:ring-emerald-400/20"
          />
        </div>

        <div class="flex items-center justify-between">
          <p class="text-sm text-slate-400">
            Changes are stored locally in your browser and applied on next reload.
          </p>
          <div class="flex gap-3">
            <button
              type="button"
              class="rounded-lg border border-slate-700 px-3 py-2 text-sm font-semibold text-slate-300 transition hover:border-slate-500"
              @click="resetToDefaults"
            >
              Reset
            </button>
            <button
              type="button"
              class="rounded-lg bg-emerald-500 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-emerald-400"
              @click="persist"
            >
              Save
            </button>
          </div>
        </div>
      </form>

      <section class="rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
        <h2 class="text-lg font-semibold text-white">Development proxy</h2>
        <p class="mt-2 text-sm text-slate-400">
          When running <code>pnpm dev</code> you can point the UI at a remote or staging agent by
          setting <code>VITE_DEV_SERVER_PROXY</code> in <code>.env.local</code>.
        </p>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'

const apiUrl = ref('')

const STORAGE_KEY = 'agentd.ui.settings'

type Settings = {
  apiUrl: string
}

onMounted(() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as Settings
      apiUrl.value = parsed.apiUrl
    }
  } catch (error) {
    console.warn('Unable to parse stored settings', error)
  }
})

function persist() {
  const payload: Settings = {
    apiUrl: apiUrl.value
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
}

function resetToDefaults() {
  localStorage.removeItem(STORAGE_KEY)
  apiUrl.value = ''
}
</script>
