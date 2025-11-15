<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <div class="text-xs text-subtle-foreground">Configure workflow step</div>
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
    </div>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Tool
      <select
        v-model="toolName"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
        :disabled="!isDesignMode || hydratingRef"
      >
        <option value="">— Select tool —</option>
        <option v-for="t in toolOptions" :key="t.name" :value="t.name">{{ t.name }}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Step Text
      <input
        v-model="stepText"
        type="text"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
        placeholder="Describe this step"
        :disabled="!isDesignMode || hydratingRef"
      />
    </label>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Guard
      <input
        v-model="guardText"
        type="text"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
        placeholder="Example: A.os != 'windows'"
        :disabled="!isDesignMode || hydratingRef"
      />
    </label>

    <label class="flex items-center gap-2 text-[11px] text-muted-foreground">
      <input v-model="publishResult" type="checkbox" class="accent-accent" :disabled="!isDesignMode || hydratingRef" />
      Publish result
    </label>

    <div v-if="parameterSchemaFiltered" class="space-y-1">
      <div class="text-[11px] font-semibold text-muted-foreground">Parameters</div>
      <ParameterFormField
        :schema="parameterSchemaFiltered"
        :model-value="argsState"
        @update:model-value="onArgsUpdate"
      />
    </div>

    <details class="mt-1" v-if="isDesignMode">
      <summary class="cursor-pointer text-[11px] text-subtle-foreground">Advanced (promote to attribute)</summary>
      <div class="mt-2 space-y-2">
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Attribute
          <input v-model="outputAttr" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="e.g. result" />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output From
          <input v-model="outputFrom" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="payload | json.<path> | delta.<key> | args.<key>" />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Value
          <input v-model="outputValue" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="Literal override" />
        </label>
      </div>
    </details>

    <div class="pt-1 flex items-center justify-end gap-2">
      <button
        class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
        :disabled="!isDirty || !isDesignMode"
        @click="applyChanges"
      >
        Apply
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type Ref } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import ParameterFormField from '@/components/flow/ParameterFormField.vue'
import type { StepNodeData } from '@/types/flow'
import type { WarppStep, WarppTool } from '@/types/warpp'

const props = defineProps<{
  nodeId: string
  data: StepNodeData
  tools: WarppTool[]
}>()

const { updateNodeData } = useVueFlow()
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))

const OUTPUT_KEYS = new Set(['output_attr', 'output_from', 'output_value'])

const isDesignMode = computed(() => modeRef.value === 'design')
const toolOptions = computed(() => {
  const options = [...(props.tools ?? [])]
  const current = props.data?.step?.tool?.name
  if (current && !options.some((t) => t.name === current)) options.push({ name: current })
  return options
})

const stepText = ref('')
const guardText = ref('')
const publishResult = ref(false)
const toolName = ref('')
const argsState = ref<Record<string, unknown>>({})
const outputAttr = ref('')
const outputFrom = ref('')
const outputValue = ref('')
const isDirty = ref(false)

const currentTool = computed(() => toolOptions.value.find((t) => t.name === toolName.value) ?? null)
const parameterSchema = computed(() => currentTool.value?.parameters ?? null)
const parameterSchemaFiltered = computed(() => {
  const schema: any = parameterSchema.value
  if (!schema || typeof schema !== 'object') return schema
  const cloned: any = { ...schema }
  if (schema.properties && typeof schema.properties === 'object') {
    cloned.properties = { ...schema.properties }
    for (const k of Object.keys(cloned.properties)) {
      if (OUTPUT_KEYS.has(k)) delete cloned.properties[k]
    }
    if (Object.keys(cloned.properties).length === 0) return null
  }
  if (Array.isArray(schema.required)) cloned.required = schema.required.filter((k: string) => !OUTPUT_KEYS.has(k))
  return cloned
})

let suppress = false
watch(
  () => props.data?.step,
  (next) => {
    suppress = true
    stepText.value = next?.text ?? ''
    guardText.value = next?.guard ?? ''
    publishResult.value = Boolean(next?.publish_result)
    toolName.value = next?.tool?.name ?? ''
    argsState.value = cloneArgs(next?.tool?.args)
    // Strip output keys
    for (const k of OUTPUT_KEYS) delete (argsState.value as any)[k]
    const a = (next?.tool?.args ?? {}) as Record<string, unknown>
    outputAttr.value = typeof a.output_attr === 'string' ? (a.output_attr as string) : ''
    outputFrom.value = typeof a.output_from === 'string' ? (a.output_from as string) : ''
    outputValue.value = typeof a.output_value === 'string' ? (a.output_value as string) : ''
    isDirty.value = false
    suppress = false
  },
  { immediate: true, deep: true },
)

watch([stepText, guardText, publishResult, toolName, outputAttr, outputFrom, outputValue], () => markDirty())
watch(argsState, () => markDirty(), { deep: true })

function onArgsUpdate(value: unknown) {
  if (value && typeof value === 'object' && !Array.isArray(value)) argsState.value = value as Record<string, unknown>
  else argsState.value = {}
  markDirty()
}

function markDirty() {
  if (suppress || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
}

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return
  const payload = buildStep()
  updateNodeData(props.nodeId, { ...(props.data ?? { order: 0 }), step: payload })
  isDirty.value = false
}

function buildStep(): WarppStep {
  const built = buildToolPayload(toolName.value, argsState.value)
  if (built) {
    const merged: Record<string, unknown> = { ...(built.args ?? {}) }
    const oa = outputAttr.value.trim()
    const of = outputFrom.value.trim()
    const ov = outputValue.value.trim()
    if (oa) merged.output_attr = oa
    if (of) merged.output_from = of
    if (ov) merged.output_value = ov
    if (Object.keys(merged).length) built.args = merged
  }
  const next: WarppStep = {
    ...(props.data?.step ?? ({} as WarppStep)),
    id: props.nodeId,
    text: stepText.value,
    guard: guardText.value.trim() ? guardText.value.trim() : undefined,
    publish_result: publishResult.value,
    tool: built,
  }
  return cloneStep(next)
}

function buildToolPayload(name: string, args: Record<string, unknown> | undefined) {
  if (!name) return undefined
  const pruned = pruneArgs(args)
  if (!pruned || (typeof pruned === 'object' && Object.keys(pruned).length === 0)) return { name }
  return { name, args: pruned as Record<string, unknown> }
}

function pruneArgs(value: unknown): unknown {
  if (value === undefined || value === null) return undefined
  if (Array.isArray(value)) return value.map((v) => pruneArgs(v)).filter((v) => v !== undefined)
  if (typeof value === 'object') {
    const result: Record<string, unknown> = {}
    Object.entries(value as Record<string, unknown>).forEach(([k, v]) => {
      const pruned = pruneArgs(v)
      if (pruned !== undefined) result[k] = pruned
    })
    return Object.keys(result).length ? result : undefined
  }
  return value
}

function cloneArgs(input: Record<string, unknown> | undefined) {
  if (!input) return {}
  try { return JSON.parse(JSON.stringify(input)) } catch { return { ...input } }
}
function cloneStep(step: Record<string, unknown>) {
  try { return JSON.parse(JSON.stringify(step)) as WarppStep } catch { return step as any }
}
</script>

