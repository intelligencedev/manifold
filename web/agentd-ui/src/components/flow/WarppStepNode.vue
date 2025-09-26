<template>
  <div
    class="relative min-w-[240px] max-w-[320px] rounded-lg border border-border/60 bg-surface/90 p-3 text-xs text-muted-foreground shadow-lg"
  >
    <Handle type="target" :position="Position.Left" class="!bg-accent" />
    <div class="flex items-start justify-between gap-2">
      <div class="flex-1">
        <div class="text-sm font-semibold text-foreground">
          {{ headerLabel }}
        </div>
      </div>
      <span class="text-[10px] uppercase tracking-wide text-faint-foreground"
        >#{{ orderLabel }}</span
      >
    </div>

    <div class="mt-3 space-y-2">
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Step Text
        <input
          v-model="stepText"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Describe this step"
          @keydown.meta.enter.prevent="applyChanges"
          @keydown.ctrl.enter.prevent="applyChanges"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Guard
        <input
          v-model="guardText"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Example: A.os != 'windows'"
          @keydown.meta.enter.prevent="applyChanges"
          @keydown.ctrl.enter.prevent="applyChanges"
        />
      </label>
      <label class="flex items-center gap-2 text-[11px] text-muted-foreground">
        <input v-model="publishResult" type="checkbox" class="accent-accent" />
        Publish result
      </label>
    </div>

    <div class="mt-3 space-y-2">
      <div class="text-[11px] font-semibold text-muted-foreground">Parameters</div>
      <ParameterFormField
        v-if="parameterSchema"
        :schema="parameterSchema"
        :model-value="argsState"
        @update:model-value="onArgsUpdate"
      />
      <p v-else-if="toolName" class="text-[11px] italic text-faint-foreground">
        This tool has no configurable parameters.
      </p>
      <p v-else class="text-[11px] italic text-faint-foreground">
        Select a tool to edit parameters.
      </p>
    </div>

    <div class="mt-4 flex items-center justify-end gap-2">
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
      <button
        class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
        :disabled="!isDirty"
        @click="applyChanges"
        title="Apply changes (Cmd/Ctrl+Enter)"
      >
        Apply
      </button>
    </div>

    <Handle type="source" :position="Position.Right" class="!bg-accent" />
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
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))

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
const isDirty = ref(false)

const orderLabel = computed(() => (props.data?.order ?? 0) + 1)

const currentTool = computed(
  () => toolOptions.value.find((tool) => tool.name === toolName.value) ?? null,
)
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

watch([stepText, guardText, publishResult, toolName], () => markDirty())
watch(argsState, () => markDirty(), { deep: true })

function markDirty() {
  if (suppressCommit || hydratingRef.value) return
  isDirty.value = true
}

function onArgsUpdate(value: unknown) {
  if (value && typeof value === 'object' && !Array.isArray(value)) argsState.value = value as Record<string, unknown>
  else argsState.value = {}
  markDirty()
}

function commit() {
  if (hydratingRef.value) {
    return
  }
  const toolPayload = buildToolPayload(toolName.value, argsState.value)
  const nextStep = {
    ...(props.data?.step ?? {}),
    id: props.id,
    text: stepText.value,
    guard: guardText.value.trim() ? guardText.value.trim() : undefined,
    publish_result: publishResult.value,
    tool: toolPayload,
  }
  // Skip update if nothing changed (shallow compare key fields + JSON fallback for args)
  const prev = props.data?.step
  if (prev) {
    const same =
      prev.text === nextStep.text &&
      prev.guard === nextStep.guard &&
      Boolean(prev.publish_result) === Boolean(nextStep.publish_result) &&
      (prev.tool?.name || '') === (nextStep.tool?.name || '') &&
      JSON.stringify(prev.tool?.args || {}) === JSON.stringify(nextStep.tool?.args || {})
    if (same) {
      return
    }
  }
  updateNodeData(props.id, { ...(props.data ?? { order: 0 }), step: cloneStep(nextStep) })
}

function applyChanges() {
  if (!isDirty.value) return
  commit()
  isDirty.value = false
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
