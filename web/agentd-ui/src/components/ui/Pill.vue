<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed, type PropType } from 'vue'

type PillTone = 'accent' | 'neutral' | 'success' | 'danger' | 'warning' | 'info'
type PillSize = 'sm' | 'md'

const props = defineProps({
  tone: { type: String as PropType<PillTone>, default: 'neutral' },
  size: { type: String as PropType<PillSize>, default: 'md' },
  glow: { type: Boolean, default: false },
})

const toneClasses: Record<PillTone, string> = {
  accent: 'bg-accent/15 text-accent border border-accent/30',
  neutral: 'bg-surface-muted/70 text-subtle-foreground border border-white/10',
  success: 'bg-success/15 text-success border border-success/30',
  danger: 'bg-danger/15 text-danger border border-danger/30',
  warning: 'bg-warning/15 text-warning border border-warning/30',
  info: 'bg-info/15 text-info border border-info/30',
}

const classes = computed(() => [
  'inline-flex items-center gap-1 rounded-full font-semibold leading-none tracking-tight',
  props.size === 'sm' ? 'px-2 py-0.5 text-[11px]' : 'px-3 py-1 text-xs',
  toneClasses[props.tone],
  props.glow ? 'pill-glow' : '',
])
</script>
