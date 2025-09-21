<template>
  <div class="flex h-screen min-h-0 flex-col overflow-hidden bg-background text-foreground">
    <header class="border-b border-border/70 bg-surface/70 backdrop-blur">
      <div class="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
        <div class="flex items-center gap-3">
          <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-accent/20">
            <span class="text-xl font-semibold text-accent">A</span>
          </span>
          <div>
            <p class="text-lg font-semibold">agentd</p>
            <p class="text-sm text-subtle-foreground">Operational intelligence dashboard</p>
          </div>
        </div>
        <nav class="hidden gap-4 text-sm font-medium md:flex">
          <RouterLink
            v-for="item in navigation"
            :key="item.to"
            :to="item.to"
            class="rounded px-3 py-2 transition hover:bg-surface-muted/60"
            active-class="bg-surface-muted text-accent"
          >
            {{ item.label }}
          </RouterLink>
        </nav>
        <div class="flex items-center gap-2">
          <span class="hidden items-center gap-2 text-sm text-subtle-foreground sm:flex">
            <span class="h-2.5 w-2.5 rounded-full bg-success"></span>
            Online
          </span>
          <ThemeToggle class="hidden sm:block" />
          <button
            type="button"
            class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold transition hover:border-border hover:text-foreground"
          >
            Refresh
          </button>
        </div>
      </div>
    </header>

    <main class="flex w-full flex-1 min-h-0 flex-col overflow-hidden px-6 py-10">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import ThemeToggle from '@/components/ThemeToggle.vue'

const navigation = [
  { label: 'Overview', to: '/' },
  { label: 'Chat', to: '/chat' },
  { label: 'Flow', to: '/flow' },
  { label: 'Runs', to: '/runs' },
  { label: 'Settings', to: '/settings' },
]
</script>
