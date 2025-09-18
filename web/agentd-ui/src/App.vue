<template>
  <div class="min-h-screen bg-slate-950 text-slate-100">
    <header class="border-b border-slate-800 bg-slate-900/70 backdrop-blur">
      <div class="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
        <div class="flex items-center gap-3">
          <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-500/20">
            <span class="text-xl font-semibold text-emerald-400">A</span>
          </span>
          <div>
            <p class="text-lg font-semibold">agentd</p>
            <p class="text-sm text-slate-400">Operational intelligence dashboard</p>
          </div>
        </div>
        <nav class="hidden gap-4 text-sm font-medium md:flex">
          <RouterLink
            v-for="item in navigation"
            :key="item.to"
            :to="item.to"
            class="rounded px-3 py-2 transition hover:bg-slate-800"
            active-class="bg-slate-800 text-emerald-400"
          >
            {{ item.label }}
          </RouterLink>
        </nav>
        <div class="flex items-center gap-2">
          <span class="flex items-center gap-2 text-sm text-slate-400">
            <span class="h-2.5 w-2.5 rounded-full bg-emerald-400"></span>
            Online
          </span>
          <button
            type="button"
            class="rounded-lg border border-slate-700 px-3 py-2 text-sm font-semibold transition hover:border-slate-500 hover:text-white"
          >
            Refresh
          </button>
        </div>
      </div>
    </header>

    <main class="mx-auto max-w-7xl px-6 py-10">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'

const navigation = [
  { label: 'Overview', to: '/' },
  { label: 'Runs', to: '/runs' },
  { label: 'Settings', to: '/settings' }
]
</script>
