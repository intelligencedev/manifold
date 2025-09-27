<template>
  <div class="flex h-screen min-h-0 flex-col overflow-hidden bg-background text-foreground">
    <header class="border-b border-border/70 bg-surface/70 backdrop-blur">
  <!-- Use a 3-column grid so left is flush left, center is perfectly centered, and right is flush right -->
  <div class="relative w-full grid grid-cols-[auto_1fr_auto] items-center gap-4 px-3 py-2">
          <!-- Left: logo (flush left) -->
          <div class="flex items-center gap-3" style="min-width:0;">
            <img :src="manifoldLogo" alt="Manifold logo" class="h-10 w-10 rounded object-contain" />
            <div class="hidden sm:block">
              <p class="text-lg font-semibold">Manifold</p>
            </div>
          </div>

          <!-- Middle: left-aligned nav (reads left-to-right) -->
          <nav aria-label="Primary" class="hidden md:flex items-center gap-2 text-sm font-medium overflow-hidden">
            <RouterLink
              v-for="item in navigation"
              :key="item.to"
              :to="item.to"
              class="inline-flex items-center justify-center min-w-[40px] min-h-[40px] rounded px-3 py-2 transition hover:bg-surface-muted/60"
              :class="$route.path === item.to || $route.path.startsWith(item.to + '/') ? 'bg-surface-muted text-accent' : ''"
              :aria-current="$route.path === item.to || $route.path.startsWith(item.to + '/') ? 'page' : undefined"
            >
              {{ item.label }}
            </RouterLink>

            <!-- More menu placeholder for overflow (implementation TBD) -->
            <div class="relative">
              <!-- will move low-priority items here when needed -->
            </div>
          </nav>

          <!-- Right: utilities cluster (status+refresh, theme icon, divider, avatar) -->
          <div class="flex items-center gap-3 justify-self-end">
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded px-2 py-2 text-sm text-subtle-foreground hover:text-foreground"
              aria-label="Connection status and refresh"
              @click="handleRefresh"
            >
              <span class="h-2.5 w-2.5 rounded-full bg-success" aria-hidden="true"></span>
              <span class="sr-only">Online</span>
              <svg class="h-4 w-4 text-subtle-foreground" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v6h6M20 20v-6h-6" />
              </svg>
            </button>

            <button @click="toggleTheme" type="button" class="inline-flex items-center justify-center rounded p-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent" aria-label="Toggle theme">
              <span aria-hidden="true">🌓</span>
            </button>

            <span class="hidden sm:block h-6 w-px bg-border/60" aria-hidden="true"></span>
            <div class="ml-1">
              <AccountButton :size="24" />
            </div>
          </div>
      </div>
    </header>

    <main class="flex w-full flex-1 min-h-0 flex-col overflow-hidden px-6 py-4">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import ThemeToggle from '@/components/ThemeToggle.vue'
import AccountButton from '@/components/AccountButton.vue'
import manifoldLogo from '@/assets/images/manifold_logo.png'

import { ref } from 'vue'

const isDark = ref(false)

function toggleTheme() {
  isDark.value = !isDark.value
  // hook into global theme handling if present
  console.log('toggle theme ->', isDark.value ? 'dark' : 'light')
}

function handleRefresh() {
  // placeholder: trigger any refresh logic (e.g., refetch queries)
  console.log('refresh clicked')
}

const navigation = [
  { label: 'Overview', to: '/' },
  { label: 'Chat', to: '/chat' },
  { label: 'Playground', to: '/playground' },
  { label: 'Flow', to: '/flow' },
  { label: 'Runs', to: '/runs' },
  { label: 'Settings', to: '/settings' },
]
</script>
