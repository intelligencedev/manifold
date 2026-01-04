<template>
  <component :is="tag" :class="panelClass">
    <header v-if="hasHeader" class="mb-4 flex items-start justify-between gap-4">
      <div class="space-y-1">
        <p v-if="eyebrow" class="text-[11px] font-semibold uppercase tracking-wide text-subtle-foreground">
          {{ eyebrow }}
        </p>
        <h2 v-if="title" class="text-lg font-semibold leading-tight">{{ title }}</h2>
        <p v-if="description" class="text-sm text-muted-foreground leading-relaxed">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </header>

    <slot />

    <footer v-if="$slots.footer" class="mt-6 border-t border-white/10 pt-4">
      <slot name="footer" />
    </footer>
  </component>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'

type PanelProps = {
  title?: string
  description?: string
  eyebrow?: string
  padded?: boolean
  as?: keyof HTMLElementTagNameMap
}

const props = defineProps<PanelProps>()
const slots = useSlots()

const tag = computed(() => props.as || 'section')

const hasHeader = computed(
  () => Boolean(props.title || props.description || props.eyebrow || slots.actions),
)

const panelClass = computed(() => [
  'glass-surface relative w-full rounded-[var(--radius-lg,26px)] border border-white/12 text-foreground shadow-[0_20px_70px_rgba(0,0,0,0.35)]',
  'supports-[backdrop-filter]:backdrop-blur-xl',
  props.padded === false ? 'p-0' : 'p-5 md:p-6',
])

const eyebrow = computed(() => props.eyebrow ?? '')
const title = computed(() => props.title ?? '')
const description = computed(() => props.description ?? '')
</script>
