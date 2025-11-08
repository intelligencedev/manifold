<template>
  <div class="aperture flex h-screen min-h-0 flex-col overflow-hidden bg-background text-foreground">
    <header>
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

      <!-- Right: utilities cluster (theme toggle, divider, project select, avatar) -->
      <div class="flex items-center gap-3 justify-self-end">
        <span class="hidden sm:block h-6 w-px bg-border/60" aria-hidden="true"></span>
        <div class="hidden sm:flex items-center gap-2">
          <DropdownSelect
            v-model="selectedProjectId"
            :options="projectOptions"
            size="sm"
            placeholder="Project"
            title="Current project"
            aria-label="Project select"
          />
        </div>
        <!-- Theme toggle: immediately left of the account dropdown -->
        <button
          type="button"
          class="ap-input flex items-center justify-center h-9 w-9 rounded-md transition-colors hover:bg-surface-muted/60 focus:outline-none"
          :aria-label="currentAppearance === 'dark' ? 'Dark theme active – switch to light' : 'Light theme active – switch to dark'"
          @click="toggleAperture"
        >
          <!-- Icon now reflects CURRENT state (moon = dark, sun = light) -->
          <component :is="currentAppearance === 'dark' ? MoonIcon : SunIcon" class="h-5 w-5" />
        </button>
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
import AccountButton from '@/components/AccountButton.vue'
import DropdownSelect from '@/components/DropdownSelect.vue'
import manifoldLogo from '@/assets/images/manifold_logo.png'

import { ref, computed, onMounted } from 'vue'
import { useProjectsStore } from '@/stores/projects'
import { useThemeStore } from '@/stores/theme'
import MoonIcon from '@/components/icons/Moon.vue'
import SunIcon from '@/components/icons/Sun.vue'

// Theme store provides resolved theme (includes system logic)
const themeStore = useThemeStore()
const currentAppearance = computed(() => themeStore.resolvedTheme.appearance)

function toggleAperture() {
  // Explicitly flip between aperture-dark and aperture-light, ignoring system & other themes.
  const next = themeStore.resolvedThemeId === 'aperture-dark' ? 'aperture-light' : 'aperture-dark'
  themeStore.setTheme(next)
}

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

// Legacy toggleTheme retained if referenced elsewhere (currently unused)
function toggleTheme() {
  toggleAperture()
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

// Projects dropdown (global project selection for all tools/views)
const projectsStore = useProjectsStore()
onMounted(() => { void projectsStore.refresh() })

const projectOptions = computed(() => {
  return projectsStore.projects.map(p => ({ id: p.id, label: p.name, value: p.id }))
})

const selectedProjectId = computed({
  get: () => projectsStore.currentProjectId || '',
  set: (v: string) => { void projectsStore.setCurrent(v) },
})
</script>
