<template>
  <section class="space-y-6">
    <header class="space-y-2">
      <h1 class="text-2xl font-semibold text-foreground">Settings</h1>
      <p class="text-sm text-subtle-foreground">
        Configure integrations, authentication, and advanced execution knobs for agentd.
      </p>
    </header>

    <div class="space-y-6">
      <form class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
        <div>
          <label class="text-sm font-medium text-muted-foreground" for="api-url"
            >API Base URL</label
          >
          <p class="text-xs text-faint-foreground">
            Used during local development when the Go backend is proxied.
          </p>
          <input
            id="api-url"
            v-model="apiUrl"
            type="url"
            placeholder="https://localhost:32180/api"
            class="mt-2 w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
          />
        </div>

        <div class="flex items-center justify-between">
          <p class="text-sm text-subtle-foreground">
            Changes are stored locally in your browser and applied on next reload.
          </p>
          <div class="flex gap-3">
            <button
              type="button"
              class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition hover:border-border"
              @click="resetToDefaults"
            >
              Reset
            </button>
            <button
              type="button"
              class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90"
              @click="persist"
            >
              Save
            </button>
          </div>
        </div>
      </form>

      <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Appearance</h2>
          <p class="text-sm text-subtle-foreground">
            Swap themes or follow your operating system. Changes apply instantly.
          </p>
        </header>
        <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <button
            v-for="option in themeOptions"
            :key="option.id"
            type="button"
            :class="[
              'flex flex-col rounded-xl border px-4 py-3 text-left shadow-sm transition',
              option.id === themeSelection
                ? 'border-accent bg-accent/10'
                : 'border-border/60 bg-surface-muted/40 hover:border-border/80 hover:bg-surface-muted/70',
            ]"
            @click="selectTheme(option.id)"
          >
            <span class="text-sm font-semibold text-foreground">{{ option.label }}</span>
            <span class="text-xs text-subtle-foreground">{{ option.description }}</span>
            <span class="text-[10px] uppercase tracking-wide text-faint-foreground">
              {{ option.id === 'system' ? 'auto' : option.appearance }}
            </span>
          </button>
        </div>
      </section>

      <section class="rounded-2xl border border-border/70 bg-surface p-6">
        <h2 class="text-lg font-semibold text-foreground">Development proxy</h2>
        <p class="mt-2 text-sm text-subtle-foreground">
          When running <code>pnpm dev</code> you can point the UI at a remote or staging agent by
          setting <code>VITE_DEV_SERVER_PROXY</code> in <code>.env.local</code>.
        </p>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useThemeStore } from '@/stores/theme'
import type { ThemeChoice } from '@/theme/themes'

const apiUrl = ref('')

const STORAGE_KEY = 'agentd.ui.settings'

type Settings = {
  apiUrl: string
}

const themeStore = useThemeStore()
const themeOptions = computed(() => themeStore.options)
const themeSelection = computed(() => themeStore.selection)

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
    apiUrl: apiUrl.value,
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
}

function resetToDefaults() {
  localStorage.removeItem(STORAGE_KEY)
  apiUrl.value = ''
}

function selectTheme(choice: ThemeChoice) {
  themeStore.setTheme(choice)
}
</script>
