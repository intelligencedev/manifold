<template>
  <section class="space-y-6 h-full">
    <header class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-foreground">Playground</h1>
        <p class="text-sm text-subtle-foreground">Experiment with prompts, datasets, and runs.</p>
      </div>
      <nav class="flex flex-wrap gap-2" aria-label="Playground navigation">
        <RouterLink
          v-for="item in items"
          :key="item.to"
          :to="item.to"
          class="rounded-lg border border-border/60 px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground"
          :class="isActive(item.to) ? 'bg-surface text-foreground border-border' : ''"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
    </header>

    <RouterView />
  </section>
</template>

<script setup lang="ts">
import { RouterLink, RouterView, useRoute } from 'vue-router'

const route = useRoute()
const items = [
  { label: 'Overview', to: '/playground' },
  { label: 'Prompts', to: '/playground/prompts' },
  { label: 'Datasets', to: '/playground/datasets' },
  { label: 'Experiments', to: '/playground/experiments' },
]

function isActive(path: string) {
  return route.path === path || route.path.startsWith(`${path}/`)
}
</script>
