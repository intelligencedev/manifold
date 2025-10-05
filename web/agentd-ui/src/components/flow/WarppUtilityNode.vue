<template>
  <div class="relative rounded-lg border border-border/60 bg-surface/90 p-3 text-xs text-muted-foreground shadow-lg min-w-[220px] max-w-[320px]">
    <Handle type="target" :position="Position.Left" class="!bg-accent" />
    <div class="flex items-start justify-between gap-2">
      <div class="flex-1">
        <div class="text-sm font-semibold text-foreground select-none">
          {{ headerLabel }}
        </div>
      </div>
      <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Utility</span>
    </div>

    <div class="mt-3 space-y-2">
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Display Label
        <input
          v-model="labelText"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Optional heading"
          :disabled="!isDesignMode"
          @input="markDirty"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Textbox Content
        <textarea
          v-if="isDesignMode"
          v-model="contentText"
          rows="4"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Enter static text or use ${A.key} placeholders"
          @input="markDirty"
        ></textarea>
        <div
          v-else
          class="min-h-[92px] rounded border border-border/60 bg-surface-muted px-2 py-2 text-[11px] text-foreground whitespace-pre-wrap break-words"
        >
          {{ runtimeText || 'Run the workflow to see resolved text.' }}
        </div>
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Output Attribute
        <input
          v-model="outputAttr"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          :placeholder="`Defaults to ${defaultAttributeHint}`"
          :disabled="!isDesignMode"
          @input="markDirty"
        />
      </label>
      <p class="text-[10px] text-faint-foreground">
        <template v-if="isDesignMode">
          When left blank the value is published as <code>{{ defaultAttributeHint }}</code>.
        </template>
        <template v-else>
          Value published as <code>{{ runtimeOutputAttr }}</code>.
        </template>
      </p>
      <p v-if="runtimeStatus === 'pending'" class="text-[10px] italic text-faint-foreground">
        Waiting for execution…
      </p>
      <p v-else-if="runtimeStatusMessage" class="text-[10px] italic text-faint-foreground">
        {{ runtimeStatusMessage }}
      </p>
      <p v-if="runtimeError && runtimeStatus !== 'pending'" class="rounded border border-danger/40 bg-danger/10 px-2 py-1 text-[10px] text-danger-foreground">
        {{ runtimeError }}
      </p>
    </div>

    <div v-if="isDesignMode" class="mt-3 flex items-center justify-end gap-2">
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
      <button
        class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
        :disabled="!isDirty"
        @click="applyChanges"
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

import type { StepNodeData } from '@/types/flow'
import type { WarppStep, WarppStepTrace } from '@/types/warpp'
import type { Ref } from 'vue'

const TOOL_NAME_FALLBACK = 'utility_textbox'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData } = useVueFlow()
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const runTraceRef = inject<Ref<Record<string, WarppStepTrace>>>('warppRunTrace', ref<Record<string, WarppStepTrace>>({}))
const runningRef = inject<Ref<boolean>>('warppRunning', ref(false))

const labelText = ref('')
const contentText = ref('')
const outputAttr = ref('')
const isDirty = ref(false)

const toolName = computed(() => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK)
const defaultAttributeHint = computed(() => `${props.id}_text`)
const headerLabel = computed(() => labelText.value.trim() || prettifyName(toolName.value))
const isDesignMode = computed(() => modeRef.value === 'design')
const runtimeTrace = computed(() => runTraceRef.value[props.id])
const runtimeText = computed(() => {
  const trace = runtimeTrace.value
  const text = trace?.renderedArgs?.text
  if (typeof text === 'string') return text
  return ''
})
const runtimeOutputAttr = computed(() => {
  const trace = runtimeTrace.value
  if (trace?.renderedArgs && typeof trace.renderedArgs.output_attr === 'string') {
    return trace.renderedArgs.output_attr
  }
  return defaultAttributeHint.value
})
const runtimeStatus = computed(() => {
  if (runtimeTrace.value?.status) return runtimeTrace.value.status
  if (modeRef.value === 'run' && runningRef.value && !runtimeTrace.value) return 'pending'
  return undefined
})
const runtimeError = computed(() => runtimeTrace.value?.error)
const runtimeStatusMessage = computed(() => {
  const trace = runtimeTrace.value
  if (!trace) return undefined
  switch (trace.status) {
    case 'skipped':
      return 'Guard prevented execution.'
    case 'noop':
      return 'Utility node did not execute.'
    case 'error':
      return 'Utility node failed.'
    default:
      return undefined
  }
})

let suppressCommit = false

watch(
  () => props.data?.step,
  (step) => {
    suppressCommit = true
    const args = (step?.tool?.args ?? {}) as Record<string, unknown>
    labelText.value = String(args.label ?? step?.text ?? '')
    contentText.value = String(args.text ?? '')
    outputAttr.value = typeof args.output_attr === 'string' ? (args.output_attr as string) : ''
    isDirty.value = false
    suppressCommit = false
  },
  { immediate: true, deep: true },
)

watch([labelText, contentText, outputAttr], () => {
  if (suppressCommit || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
})

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return
  commit()
  isDirty.value = false
}

function markDirty() {
  if (suppressCommit || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
}

function commit() {
  if (hydratingRef.value || !isDesignMode.value) return
  const args = buildArgs()
  const nextStep: WarppStep = {
    ...(props.data?.step ?? ({} as WarppStep)),
    id: props.id,
    text: labelText.value.trim() || prettifyName(toolName.value),
    publish_result: Boolean(props.data?.step?.publish_result),
    tool: {
      name: toolName.value,
      args,
    },
  }
  updateNodeData(props.id, { ...(props.data ?? { order: 0, kind: 'utility' }), step: cloneStep(nextStep) })
}

function buildArgs(): Record<string, unknown> {
  const args: Record<string, unknown> = {}
  const label = labelText.value.trim()
  const text = contentText.value
  const attr = outputAttr.value.trim()
  if (label) args.label = label
  if (text) args.text = text
  if (attr) args.output_attr = attr
  return args
}

function cloneStep(step: WarppStep) {
  try {
    return JSON.parse(JSON.stringify(step)) as WarppStep
  } catch (err) {
    console.warn('Failed to clone utility step', err)
    return { ...step }
  }
}

function prettifyName(name: string): string {
  if (!name.startsWith('utility_')) return name
  return name
    .slice('utility_'.length)
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (ch) => ch.toUpperCase())
}
</script>
