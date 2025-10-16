<template>
  <div class="flex h-screen min-h-0 flex-col overflow-hidden bg-background text-foreground">
    <header class="border-b border-border/70 bg-surface/70 backdrop-blur">
      <!-- Use a 3-column grid so left is flush left, center is perfectly centered, and right is flush right -->
  <div class="relative w-full grid grid-cols-[auto_1fr_auto] items-center px-4 py-2">
      <!-- Left: logo + app name (flush left) -->
      <div class="flex items-center gap-3" style="min-width:0;">
        <img :src="manifoldLogo" alt="Manifold logo" class="h-10 w-10 rounded object-contain" />
        <span class="text-lg font-semibold leading-none">Manifold</span>
      </div>

  <!-- Middle: centered nav (absolutely centered on md+ screens) -->
  <nav aria-label="Primary" class="hidden md:absolute md:left-1/2 md:-translate-x-1/2 md:inset-y-0 md:flex items-center justify-center gap-2 text-sm font-medium overflow-hidden">
        <RouterLink v-for="item in navigation" :key="item.to" :to="item.to"
        class="inline-flex items-center justify-center min-w-[40px] min-h-[40px] rounded px-3 py-2 transition hover:bg-surface-muted/60"
        :class="$route.path === item.to || $route.path.startsWith(item.to + '/') ? 'bg-surface-muted text-accent' : ''"
        :aria-current="$route.path === item.to || $route.path.startsWith(item.to + '/') ? 'page' : undefined">
        {{ item.label }}
        </RouterLink>

        <!-- More menu placeholder for overflow (implementation TBD) -->
        <div class="relative">
        <!-- will move low-priority items here when needed -->
        </div>
      </nav>

      <!-- Right: utilities cluster (divider, avatar) -->
      <div class="flex items-center gap-3 justify-self-end">
        <span class="hidden sm:block h-6 w-px bg-border/60" aria-hidden="true"></span>
        <div class="ml-1">
        <AccountButton :username="user?.name || user?.email" />
        </div>
      </div>
      </div>
    </header>

    <main class="flex w-full flex-1 min-h-0 flex-col overflow-hidden px-4 py-4">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import ThemeToggle from '@/components/ThemeToggle.vue'
import AccountButton from '@/components/AccountButton.vue'
import manifoldLogo from '@/assets/images/manifold_logo.png'

import { ref, computed, onMounted } from 'vue'

const isDark = ref(false)

// Load current user; fall back to global if present
const user = ref<{ name?: string; email?: string; picture?: string } | null>(null)
onMounted(async () => {
  try {
    const res = await fetch('/api/me', { credentials: 'include' })
    if (res.ok) user.value = await res.json()
    else {
      const g = (window as any).__MANIFOLD_USER__
      if (g) user.value = g
    }
  } catch (_) {
    const g = (window as any).__MANIFOLD_USER__
    if (g) user.value = g
  }
})

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
  { label: 'Projects', to: '/projects' },
  { label: 'Specialists', to: '/specialists' },
  { label: 'Chat', to: '/chat' },
  { label: 'Playground', to: '/playground' },
  { label: 'Flow', to: '/flow' },
  { label: 'Runs', to: '/runs' },
  { label: 'Settings', to: '/settings' },
]
</script>
