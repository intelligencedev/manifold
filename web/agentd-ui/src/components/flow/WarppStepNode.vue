<template>
  <div
    class="relative rounded-lg border border-border/60 bg-surface/90 p-3 text-xs text-muted-foreground shadow-lg"
    :class="collapsed ? 'min-w-[160px] w-[220px]' : 'min-w-[240px] w-[320px]'"
  >
    <Handle type="target" :position="Position.Left" class="!bg-accent" />
    <div class="flex items-start justify-between gap-2">
      <div class="flex-1">
        <div class="flex items-center gap-2">
          <button
            class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
            :aria-expanded="!collapsed"
            :title="collapsed ? 'Expand' : 'Collapse'"
            @click.prevent.stop="toggleCollapsed"
          >
            <!-- chevron icon -->
            <svg
              class="h-3.5 w-3.5 transition-transform"
              :class="collapsed ? '-rotate-90' : 'rotate-0'"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="6 9 12 15 18 9"></polyline>
            </svg>
          </button>
          <div class="text-sm font-semibold text-foreground select-none">
            {{ headerLabel }}
          </div>
        </div>
      </div>
      <span v-show="!collapsed" class="text-[10px] uppercase tracking-wide text-faint-foreground">#{{ orderLabel }}</span>
    </div>

    <div v-show="!collapsed" class="mt-3 space-y-2">
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Step Text
        <input
          v-model="stepText"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Describe this step"
          :disabled="!isDesignMode"
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
          :disabled="!isDesignMode"
          @keydown.meta.enter.prevent="applyChanges"
          @keydown.ctrl.enter.prevent="applyChanges"
        />
      </label>
      <label class="flex items-center gap-2 text-[11px] text-muted-foreground">
        <input v-model="publishResult" type="checkbox" class="accent-accent" :disabled="!isDesignMode" />
        Publish result
      </label>
    </div>

    <div v-show="!collapsed" class="mt-3 space-y-2">
      <div class="text-[11px] font-semibold text-muted-foreground">Parameters</div>
      <ParameterFormField
        v-if="isDesignMode && parameterSchema"
        :schema="parameterSchema"
        :model-value="argsState"
        @update:model-value="onArgsUpdate"
      />
      <p v-else-if="isDesignMode && toolName" class="text-[11px] italic text-faint-foreground">
        This tool has no configurable parameters.
      </p>
      <p v-else-if="isDesignMode" class="text-[11px] italic text-faint-foreground">
        Select a tool to edit parameters.
      </p>
      <div v-else class="space-y-1 text-[11px] text-muted-foreground">
        <template v-if="runtimeArgs.length">
          <div v-for="([key, value], index) in runtimeArgs" :key="`${key}-${index}`" class="flex items-start gap-2">
            <span class="min-w-[72px] font-semibold text-foreground">{{ key }}</span>
            <span class="block max-h-[6rem] overflow-hidden whitespace-pre-wrap break-words text-foreground/80">
              {{ formatRuntimeValue(value) }}
            </span>
          </div>
        </template>
        <p v-else-if="runtimeStatus === 'pending'" class="italic text-faint-foreground">
          Waiting for execution…
        </p>
        <p v-else class="italic text-faint-foreground">
          Run the workflow to see resolved values.
        </p>
        <p v-if="runtimeStatusMessage" class="italic text-faint-foreground">{{ runtimeStatusMessage }}</p>
      </div>
      <p v-if="runtimeError && runtimeStatus !== 'pending'" class="rounded border border-danger/40 bg-danger/10 px-2 py-1 text-[10px] text-danger-foreground">
        <span class="block max-h-[6rem] overflow-hidden whitespace-pre-wrap break-words">{{ runtimeError }}</span>
      </p>
    </div>

    <div v-show="!collapsed && !isDesignMode && hasRuntimeDetails" class="mt-3 flex items-center justify-end">
      <button
        type="button"
        class="text-[11px] font-medium text-accent underline decoration-dotted underline-offset-2 transition hover:text-accent-foreground"
        @click="viewRuntimeDetails"
      >
        View details
      </button>
    </div>

    <div v-show="!collapsed && isDesignMode" class="mt-4 flex items-center justify-end gap-2">
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
import type { WarppTool, WarppStepTrace } from '@/types/warpp'
import type { Ref } from 'vue'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData } = useVueFlow()

const toolsRef = inject<Ref<WarppTool[]>>('warppTools', ref<WarppTool[]>([]))
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const runTraceRef = inject<Ref<Record<string, WarppStepTrace>>>('warppRunTrace', ref<Record<string, WarppStepTrace>>({}))
const runningRef = inject<Ref<boolean>>('warppRunning', ref(false))
const openResultModal = inject<(stepId: string, title: string) => void>('warppOpenResultModal', () => {})

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
const collapsed = ref(false)

const orderLabel = computed(() => (props.data?.order ?? 0) + 1)
const isDesignMode = computed(() => modeRef.value === 'design')
const runtimeTrace = computed(() => {
  const rec = runTraceRef.value
  if (!rec || typeof rec !== 'object') return undefined
  return rec[props.id]
})
const runtimeArgs = computed(() => {
  const trace = runtimeTrace.value
  if (!trace?.renderedArgs) {
    return [] as Array<[string, unknown]>
  }
  return Object.entries(trace.renderedArgs as Record<string, unknown>)
})
const runtimeError = computed(() => runtimeTrace.value?.error)
const runtimeStatus = computed(() => {
  if (runtimeTrace.value?.status) return runtimeTrace.value.status
  if (modeRef.value === 'run' && runningRef.value && !runtimeTrace.value) return 'pending'
  return undefined
})
const runtimeStatusMessage = computed(() => {
  const trace = runtimeTrace.value
  if (!trace) return undefined
  switch (trace.status) {
    case 'skipped':
      return 'Guard prevented execution.'
    case 'noop':
      return 'Step has no tool configured.'
    case 'error':
      return 'Step encountered an error.'
    default:
      return undefined
  }
})
const hasRuntimeDetails = computed(() => Boolean(runtimeTrace.value))

const currentTool = computed(
  () => toolOptions.value.find((tool) => tool.name === toolName.value) ?? null,
)
const parameterSchema = computed(() => currentTool.value?.parameters ?? null)

const headerLabel = computed(() => currentTool.value?.name ?? 'Workflow Step')

let suppressCommit = false
let suppressToolReset = false

const MAX_PREVIEW_CHARS = 160

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
  if (suppressCommit || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
}

function onArgsUpdate(value: unknown) {
  if (value && typeof value === 'object' && !Array.isArray(value)) argsState.value = value as Record<string, unknown>
  else argsState.value = {}
  markDirty()
}

function commit() {
  if (hydratingRef.value || !isDesignMode.value) {
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
  if (!isDesignMode.value || !isDirty.value) return
  commit()
  isDirty.value = false
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value
}

function formatRuntimeValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') {
    return value.length > MAX_PREVIEW_CHARS ? value.slice(0, MAX_PREVIEW_CHARS) + '…' : value
  }
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    const serialized = JSON.stringify(value)
    if (serialized.length > MAX_PREVIEW_CHARS) {
      return serialized.slice(0, MAX_PREVIEW_CHARS) + '…'
    }
    return serialized
  } catch (err) {
    console.warn('Failed to stringify runtime value', err)
    const fallback = String(value)
    return fallback.length > MAX_PREVIEW_CHARS ? fallback.slice(0, MAX_PREVIEW_CHARS) + '…' : fallback
  }
}

function viewRuntimeDetails() {
  if (!runtimeTrace.value) return
  openResultModal(props.id, headerLabel.value)
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
