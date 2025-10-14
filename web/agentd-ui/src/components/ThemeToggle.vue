<template>
  <DropdownSelect
    id="theme-toggle"
    v-model="selected"
    :options="dropdownOptions"
    size="sm"
    aria-label="Theme"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'
import type { ThemeChoice } from '@/theme/themes'
import type { DropdownOption } from '@/types/dropdown'
import DropdownSelect from './DropdownSelect.vue'

const themeStore = useThemeStore()

const dropdownOptions = computed<DropdownOption[]>(() =>
  themeStore.options.map((option) => ({
    id: option.id,
    label: option.label,
    description: option.description,
    value: option.id,
  }))
)

const selected = computed<ThemeChoice>({
  get: () => themeStore.selection,
  set: (value) => themeStore.setTheme(value),
})
</script>
