<template>
  <div class="relative min-w-[180px] max-w-[260px] rounded-lg border border-slate-700 bg-slate-900/90 p-3 text-xs text-slate-200 shadow-lg">
    <Handle type="target" :position="Position.Top" class="!bg-blue-500" />
    <div class="flex items-start justify-between gap-2">
      <div class="text-sm font-semibold text-white">
        {{ headerLabel }}
      </div>
      <span class="text-[10px] uppercase tracking-wide text-slate-500">#{{ orderLabel }}</span>
    </div>
    <p v-if="step.text" class="mt-1 text-[11px] text-slate-400">
      {{ step.text }}
    </p>
    <div v-if="parameterSource" class="mt-3 space-y-1">
      <div class="text-[11px] font-semibold text-slate-300">Parameters</div>
      <ParameterTree :value="parameterSource" class="text-left" />
    </div>
    <p v-else class="mt-3 text-[11px] italic text-slate-500">No parameters defined</p>
    <Handle type="source" :position="Position.Bottom" class="!bg-blue-500" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position, type NodeProps } from '@vue-flow/core'

import ParameterTree from '@/components/flow/ParameterTree.vue'
import type { StepNodeData } from '@/types/flow'

const props = defineProps<NodeProps<StepNodeData>>()

const step = computed(() => props.data?.step)

const headerLabel = computed(() => step.value?.tool?.name ?? 'Tool step')

const orderLabel = computed(() => (props.data?.order ?? 0) + 1)

const parameterSource = computed(() => props.data?.toolDefinition?.parameters ?? step.value?.tool?.args ?? null)
</script>
