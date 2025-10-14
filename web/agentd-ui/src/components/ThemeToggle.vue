<template>
  <div class="relative">
    <label class="sr-only" for="theme-toggle">Theme</label>
    <select
      id="theme-toggle"
      v-model="selected"
      class="appearance-none rounded-lg border border-border bg-surface-muted/60 pl-3 pr-8 py-2 text-xs font-semibold text-foreground shadow-sm transition focus:border-ring focus:outline-none focus:ring-2 focus:ring-ring/60"
    >
      <option v-for="option in options" :key="option.id" :value="option.id">
        {{ option.label }}
      </option>
    </select>
    <span
      class="pointer-events-none absolute right-5 top-1/2 -translate-y-1/2 text-[10px] text-subtle-foreground"
      >▼</span
    >
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'
import type { ThemeChoice } from '@/theme/themes'

const themeStore = useThemeStore()

const options = computed(() => themeStore.options)

const selected = computed<ThemeChoice>({
  get: () => themeStore.selection,
  set: (value) => themeStore.setTheme(value),
})
</script>
