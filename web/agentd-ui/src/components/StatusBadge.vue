<template>
  <span
    :class="[
      'inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-wide',
      statusClasses
    ]"
  >
    <span :class="['h-2.5 w-2.5 rounded-full', dotClass]"></span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ state: 'online' | 'offline' | 'degraded' | 'running' | 'failed' | 'completed' }>()

const statusClasses = computed(() => {
  switch (props.state) {
    case 'online':
    case 'completed':
      return 'bg-emerald-500/10 text-emerald-300 border border-emerald-500/20'
    case 'running':
      return 'bg-sky-500/10 text-sky-300 border border-sky-500/20'
    case 'degraded':
      return 'bg-amber-500/10 text-amber-300 border border-amber-500/20'
    case 'failed':
    case 'offline':
    default:
      return 'bg-rose-500/10 text-rose-300 border border-rose-500/20'
  }
})

const dotClass = computed(() => {
  switch (props.state) {
    case 'online':
    case 'completed':
      return 'bg-emerald-400'
    case 'running':
      return 'bg-sky-400 animate-pulse'
    case 'degraded':
      return 'bg-amber-400 animate-pulse'
    case 'failed':
    case 'offline':
    default:
      return 'bg-rose-400'
  }
})
</script>
