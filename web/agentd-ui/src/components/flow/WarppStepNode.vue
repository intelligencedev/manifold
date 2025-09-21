<template>
  <div class="relative min-w-[240px] max-w-[320px] rounded-lg border border-slate-700 bg-slate-900/90 p-3 text-xs text-slate-200 shadow-lg">
    <Handle type="target" :position="Position.Left" class="!bg-blue-500" />
    <div class="flex items-start justify-between gap-2">
      <div class="flex-1">
        <div class="text-sm font-semibold text-white">
          {{ headerLabel }}
        </div>
        <label class="mt-1 flex flex-col gap-1 text-[11px] text-slate-300">
          <span class="text-[10px] uppercase tracking-wide text-slate-500">Tool</span>
          <select
            v-model="toolName"
            class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          >
            <option value="">(none)</option>
            <option
              v-for="option in toolOptions"
              :key="option.name"
              :value="option.name"
            >
              {{ option.name }}
            </option>
          </select>
        </label>
      </div>
      <span class="text-[10px] uppercase tracking-wide text-slate-500">#{{ orderLabel }}</span>
    </div>

    <div class="mt-3 space-y-2">
      <label class="flex flex-col gap-1 text-[11px] text-slate-300">
        Step Text
        <input
          v-model="stepText"
          type="text"
          class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          placeholder="Describe this step"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-slate-300">
        Guard
        <input
          v-model="guardText"
          type="text"
          class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          placeholder="Example: A.os != 'windows'"
        />
      </label>
      <label class="flex items-center gap-2 text-[11px] text-slate-300">
        <input v-model="publishResult" type="checkbox" />
        Publish result
      </label>
    </div>

    <div class="mt-3 space-y-2">
      <div class="text-[11px] font-semibold text-slate-300">Parameters</div>
      <ParameterFormField
        v-if="parameterSchema"
        :schema="parameterSchema"
        :model-value="argsState"
        @update:modelValue="onArgsUpdate"
      />
      <p v-else-if="toolName" class="text-[11px] italic text-slate-500">
        This tool has no configurable parameters.
      </p>
      <p v-else class="text-[11px] italic text-slate-500">Select a tool to edit parameters.</p>
    </div>

    <Handle type="source" :position="Position.Right" class="!bg-blue-500" />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import { Handle, Position, useVueFlow, type NodeProps } from '@vue-flow/core'

import ParameterFormField from '@/components/flow/ParameterFormField.vue'
import type { StepNodeData } from '@/types/flow'
import type { WarppTool } from '@/types/warpp'
import type { Ref } from 'vue'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData } = useVueFlow()

const toolsRef = inject<Ref<WarppTool[]>>('warppTools', ref<WarppTool[]>([]))

const toolOptions = computed(() => {
  const options = [...(toolsRef?.value ?? [])]
  const current = props.data?.step?.tool?.name
  if (current && !options.some((tool) => tool.name === current)) {
    options.push({ name: current })
  }
  return options
})

const stepText = ref('')
const guardText = ref('')
const publishResult = ref(false)
const toolName = ref('')
const argsState = ref<Record<string, unknown>>({})

const orderLabel = computed(() => (props.data?.order ?? 0) + 1)

const currentTool = computed(() => toolOptions.value.find((tool) => tool.name === toolName.value) ?? null)
const parameterSchema = computed(() => currentTool.value?.parameters ?? null)

const headerLabel = computed(() => currentTool.value?.name ?? 'Workflow Step')

let suppressCommit = false
let suppressToolReset = false

watch(
  () => props.data?.step,
  (nextStep) => {
    suppressCommit = true
    suppressToolReset = true
    stepText.value = nextStep?.text ?? ''
    guardText.value = nextStep?.guard ?? ''
    publishResult.value = Boolean(nextStep?.publish_result)
    toolName.value = nextStep?.tool?.name ?? ''
    argsState.value = cloneArgs(nextStep?.tool?.args)
    suppressCommit = false
  },
  { immediate: true, deep: true },
)

watch(stepText, commitIfNeeded)
watch(guardText, commitIfNeeded)
watch(publishResult, commitIfNeeded)
watch(toolName, (next, prev) => {
  if (suppressCommit) {
    return
  }
  if (suppressToolReset) {
    suppressToolReset = false
    commit()
    return
  }
  if (next !== prev) {
    argsState.value = {}
  }
  commit()
})
watch(
  argsState,
  () => {
    commitIfNeeded()
  },
  { deep: true },
)

function commitIfNeeded() {
  if (suppressCommit) {
    return
  }
  commit()
}

function onArgsUpdate(value: unknown) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    argsState.value = value as Record<string, unknown>
  } else {
    argsState.value = {}
  }
}

function commit() {
  const toolPayload = buildToolPayload(toolName.value, argsState.value)
  const nextStep = {
    ...(props.data?.step ?? {}),
    id: props.id,
    text: stepText.value,
    guard: guardText.value.trim() ? guardText.value.trim() : undefined,
    publish_result: publishResult.value,
    tool: toolPayload,
  }

  updateNodeData(props.id, {
    ...(props.data ?? { order: 0 }),
    step: cloneStep(nextStep),
  })
}

function buildToolPayload(name: string, args: Record<string, unknown>) {
  if (!name) {
    return undefined
  }
  const pruned = pruneArgs(args)
  if (!pruned || (typeof pruned === 'object' && Object.keys(pruned).length === 0)) {
    return { name }
  }
  return { name, args: pruned as Record<string, unknown> }
}

function pruneArgs(value: unknown): unknown {
  if (value === undefined || value === null) {
    return undefined
  }
  if (Array.isArray(value)) {
    const prunedArray = value.map((item) => pruneArgs(item)).filter((item) => item !== undefined)
    return prunedArray.length ? prunedArray : undefined
  }
  if (typeof value === 'object') {
    const result: Record<string, unknown> = {}
    Object.entries(value as Record<string, unknown>).forEach(([key, val]) => {
      const pruned = pruneArgs(val)
      if (pruned !== undefined) {
        result[key] = pruned
      }
    })
    return Object.keys(result).length ? result : undefined
  }
  return value
}

function cloneArgs(input: Record<string, unknown> | undefined) {
  if (!input) {
    return {}
  }
  try {
    return JSON.parse(JSON.stringify(input)) as Record<string, unknown>
  } catch (err) {
    console.warn('Failed to clone args', err)
    return { ...input }
  }
}

function cloneStep(step: Record<string, unknown>) {
  try {
    return JSON.parse(JSON.stringify(step))
  } catch (err) {
    console.warn('Failed to clone step', err)
    return { ...step }
  }
}
</script>
