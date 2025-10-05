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
          @input="markDirty"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Textbox Content
        <textarea
          v-model="contentText"
          rows="4"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          placeholder="Enter static text or use ${A.key} placeholders"
          @input="markDirty"
        ></textarea>
      </label>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        Output Attribute
        <input
          v-model="outputAttr"
          type="text"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          :placeholder="`Defaults to ${defaultAttributeHint}`"
          @input="markDirty"
        />
      </label>
      <p class="text-[10px] text-faint-foreground">
        When left blank the value is published as <code>{{ defaultAttributeHint }}</code>.
      </p>
    </div>

    <div class="mt-3 flex items-center justify-end gap-2">
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
import type { WarppStep } from '@/types/warpp'
import type { Ref } from 'vue'

const TOOL_NAME_FALLBACK = 'utility_textbox'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData } = useVueFlow()
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))

const labelText = ref('')
const contentText = ref('')
const outputAttr = ref('')
const isDirty = ref(false)

const toolName = computed(() => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK)
const defaultAttributeHint = computed(() => `${props.id}_text`)
const headerLabel = computed(() => labelText.value.trim() || prettifyName(toolName.value))

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
  if (suppressCommit || hydratingRef.value) return
  isDirty.value = true
})

function applyChanges() {
  if (!isDirty.value) return
  commit()
  isDirty.value = false
}

function markDirty() {
  if (suppressCommit || hydratingRef.value) return
  isDirty.value = true
}

function commit() {
  if (hydratingRef.value) return
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
