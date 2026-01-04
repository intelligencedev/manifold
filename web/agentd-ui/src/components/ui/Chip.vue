<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed, type PropType } from 'vue'

type ChipSize = 'sm' | 'md'

const props = defineProps({
  size: { type: String as PropType<ChipSize>, default: 'sm' },
  muted: { type: Boolean, default: false },
})

const classes = computed(() => [
  'inline-flex items-center rounded-full border text-subtle-foreground',
  props.muted ? 'bg-surface-muted/40 border-border/60' : 'bg-surface-muted/70 border-border/80',
  props.size === 'md' ? 'px-3 py-1 text-sm' : 'px-2.5 py-1 text-xs',
])
</script>
