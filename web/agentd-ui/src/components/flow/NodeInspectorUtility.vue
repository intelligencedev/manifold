<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <div class="text-xs text-subtle-foreground">Configure utility</div>
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
    </div>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Display Label
      <input v-model="labelText" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="Optional heading" :disabled="!isDesignMode || hydratingRef" />
    </label>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Textbox Content
      <textarea v-model="contentText" rows="4" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto w-full h-[92px] resize-none whitespace-pre-wrap break-words" placeholder="Enter static text or use ${A.key} placeholders" :disabled="!isDesignMode || hydratingRef" />
    </label>

    <details class="mt-1" v-if="isDesignMode">
      <summary class="cursor-pointer text-[11px] text-subtle-foreground">Advanced (promote to attribute)</summary>
      <div class="mt-2 space-y-2">
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Attribute
          <input v-model="outputAttr" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" :placeholder="`Defaults to ${defaultAttributeHint}`" />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output From
          <input v-model="outputFrom" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="payload | json.<path> | delta.<key> | args.<key>" />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Value
          <input v-model="outputValue" type="text" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground" placeholder="Literal override" />
        </label>
        <p class="text-[10px] text-faint-foreground">When left blank the value is published as <code>{{ defaultAttributeHint }}</code>.</p>
      </div>
    </details>

    <div class="pt-1 flex items-center justify-end gap-2">
      <button class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40" :disabled="!isDirty || !isDesignMode" @click="applyChanges">Apply</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type Ref } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import type { StepNodeData } from '@/types/flow'
import type { WarppStep } from '@/types/warpp'

const TOOL_NAME_FALLBACK = 'utility_textbox'

const props = defineProps<{ nodeId: string; data: StepNodeData }>()
const { updateNodeData } = useVueFlow()
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))

const isDesignMode = computed(() => modeRef.value === 'design')
const labelText = ref('')
const contentText = ref('')
const outputAttr = ref('')
const outputFrom = ref('')
const outputValue = ref('')
const isDirty = ref(false)

const toolName = computed(() => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK)
const defaultAttributeHint = computed(() => `${props.nodeId}_text`)

let suppress = false
watch(
  () => props.data?.step,
  (step) => {
    suppress = true
    const args = (step?.tool?.args ?? {}) as Record<string, unknown>
    labelText.value = String(args.label ?? step?.text ?? '')
    contentText.value = String(args.text ?? '')
    outputAttr.value = typeof args.output_attr === 'string' ? (args.output_attr as string) : ''
    outputFrom.value = typeof args.output_from === 'string' ? (args.output_from as string) : ''
    outputValue.value = typeof args.output_value === 'string' ? (args.output_value as string) : ''
    isDirty.value = false
    suppress = false
  },
  { immediate: true, deep: true },
)

watch([labelText, contentText, outputAttr, outputFrom, outputValue], () => {
  if (suppress || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
})

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return
  const nextStep: WarppStep = {
    ...(props.data?.step ?? ({} as WarppStep)),
    id: props.nodeId,
    text: labelText.value.trim() || prettifyName(toolName.value),
    publish_result: Boolean(props.data?.step?.publish_result),
    tool: {
      name: toolName.value,
      args: buildArgs(),
    },
  }
  updateNodeData(props.nodeId, { ...(props.data ?? { order: 0, kind: 'utility' }), step: cloneStep(nextStep) })
  isDirty.value = false
}

function buildArgs(): Record<string, unknown> {
  const args: Record<string, unknown> = {}
  const label = labelText.value.trim()
  const text = contentText.value
  const attr = outputAttr.value.trim()
  const from = outputFrom.value.trim()
  const val = outputValue.value.trim()
  if (label) args.label = label
  if (text) args.text = text
  if (attr) args.output_attr = attr
  if (from) args.output_from = from
  if (val) args.output_value = val
  return args
}

function prettifyName(name: string): string {
  if (!name.startsWith('utility_')) return name
  return name.slice('utility_'.length).replace(/[_-]+/g, ' ').replace(/\b\w/g, (ch) => ch.toUpperCase())
}

function cloneStep(step: WarppStep) {
  try { return JSON.parse(JSON.stringify(step)) as WarppStep } catch { return { ...step } }
}
</script>

