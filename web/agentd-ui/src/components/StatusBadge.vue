<template>
  <span
    :class="[
      'inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-wide',
      statusClasses,
    ]"
  >
    <span :class="['h-2.5 w-2.5 rounded-full', dotClass]"></span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  state: 'online' | 'offline' | 'degraded' | 'running' | 'failed' | 'completed'
}>()

const statusClasses = computed(() => {
  switch (props.state) {
    case 'online':
    case 'completed':
      return 'border border-success/40 bg-success/10 text-success'
    case 'running':
      return 'border border-info/40 bg-info/10 text-info'
    case 'degraded':
      return 'border border-warning/40 bg-warning/10 text-warning'
    case 'failed':
    case 'offline':
    default:
      return 'border border-danger/40 bg-danger/10 text-danger'
  }
})

const dotClass = computed(() => {
  switch (props.state) {
    case 'online':
    case 'completed':
      return 'bg-success'
    case 'running':
      return 'animate-pulse bg-info'
    case 'degraded':
      return 'animate-pulse bg-warning'
    case 'failed':
    case 'offline':
    default:
      return 'bg-danger'
  }
})
</script>
