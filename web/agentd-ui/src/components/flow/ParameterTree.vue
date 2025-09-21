<template>
  <div :style="{ paddingLeft: paddingLeft }" class="space-y-1">
    <div v-if="label" class="text-xs font-semibold text-slate-200">
      {{ label }}
    </div>
    <div v-if="isLeaf" class="text-xs text-slate-400 break-words">{{ formattedValue }}</div>
    <div v-else class="space-y-1">
      <ParameterTree
        v-for="entry in entries"
        :key="entry.key"
        :label="entry.key"
        :value="entry.value"
        :depth="depth + 1"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

defineOptions({ name: 'ParameterTree' })

const props = withDefaults(
  defineProps<{
    value: unknown
    label?: string
    depth?: number
  }>(),
  {
    label: undefined,
    depth: 0,
  },
)

const isObject = computed(() => typeof props.value === 'object' && props.value !== null)

const entries = computed(() => {
  if (!isObject.value) {
    return [] as Array<{ key: string; value: unknown }>
  }
  if (Array.isArray(props.value)) {
    return props.value.map((value, idx) => ({ key: String(idx), value }))
  }
  return Object.entries(props.value as Record<string, unknown>).map(([key, value]) => ({ key, value }))
})

const isLeaf = computed(() => entries.value.length === 0)

const formattedValue = computed(() => {
  if (typeof props.value === 'string') return props.value
  if (typeof props.value === 'number' || typeof props.value === 'boolean') return String(props.value)
  if (props.value === null) return 'null'
  if (props.value === undefined) return 'undefined'
  return JSON.stringify(props.value, null, 2)
})

const paddingLeft = computed(() => `${Math.min(props.depth ?? 0, 6) * 12}px`)
</script>
