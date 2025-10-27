<template>
  <div class="relative flip-root text-xs text-muted-foreground min-w-[220px] w-[320px]">
    <Handle type="target" :position="Position.Left" class="!bg-accent" />

    <!-- Entire card flips -->
    <div class="flip-card" :class="showBack ? 'is-flipped' : ''">
      <!-- FRONT FACE -->
      <div class="flip-face flip-front relative rounded-lg border border-border/60 bg-surface/90 p-3 shadow-lg">
        <!-- Header with gear -->
        <div class="flex items-start justify-between gap-2">
          <div class="flex-1">
            <div class="text-sm font-semibold text-foreground select-none">
              {{ headerLabel }}
            </div>
          </div>
          <div class="flex items-center gap-1">
            <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Utility</span>
            <button
              class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
              title="Configure output"
              aria-label="Configure output"
              @click.prevent.stop="toggleBack(true)"
            >
              <GearIcon class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>

        <!-- Front content -->
        <div class="mt-3" :class="collapsed ? 'hidden' : ''">
          <div class="space-y-2">
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
                class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto"
                placeholder="Enter static text or use ${A.key} placeholders"
                @input="markDirty"
              >
              </textarea>
              <div
                v-else
                class="min-h-[92px] rounded border border-border/60 bg-surface-muted px-2 py-2 text-[11px] text-foreground whitespace-pre-wrap break-words"
              >
                <span class="block max-h-[12rem] overflow-auto">{{ runtimeText || 'Run the workflow to see resolved text.' }}</span>
              </div>
            </label>
            <p v-if="runtimeStatus === 'pending'" class="text-[10px] italic text-faint-foreground">
              Waiting for execution…
            </p>
            <p v-else-if="runtimeStatusMessage" class="text-[10px] italic text-faint-foreground">
              {{ runtimeStatusMessage }}
            </p>
            <p v-if="runtimeError && runtimeStatus !== 'pending'" class="rounded border border-danger/40 bg-danger/10 px-2 py-1 text-[10px] text-danger-foreground">
              <span class="block max-h-[6rem] overflow-auto whitespace-pre-wrap break-words">{{ runtimeError }}</span>
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

          <div v-if="!isDesignMode && hasRuntimeDetails" class="mt-3 flex items-center justify-end">
            <button
              type="button"
              class="text-[11px] font-medium text-accent underline decoration-dotted underline-offset-2 transition hover:text-accent-foreground"
              @click="viewRuntimeDetails"
            >
              View details
            </button>
          </div>
        </div>
      </div>

      <!-- BACK FACE -->
      <div class="flip-face flip-back absolute inset-0 rounded-lg border border-border/60 bg-surface/90 p-3 shadow-lg" :class="showBack ? 'pointer-events-auto' : 'pointer-events-none'">
        <div class="flex items-start justify-between gap-2">
          <button
            class="inline-flex items-center rounded px-2 py-0.5 text-[11px] text-foreground hover:bg-muted/70"
            title="Back"
            @click.prevent.stop="toggleBack(false)"
          >
            Back
          </button>
          <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Output</span>
        </div>
        <div class="mt-3" :class="collapsed ? 'hidden' : ''">
          <div class="space-y-2">
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
            <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
              Output From
              <input
                v-model="outputFrom"
                type="text"
                class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
                placeholder="payload | delta.key | args.key"
                :disabled="!isDesignMode"
                @input="markDirty"
              />
            </label>
            <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
              Output Value
              <input
                v-model="outputValue"
                type="text"
                class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
                placeholder="Literal override"
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
            <div v-if="isDesignMode" class="pt-1 flex items-center justify-end gap-2">
              <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
              <button
                class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
                :disabled="!isDirty"
                @click="applyChanges"
              >
                Apply
              </button>
            </div>
          </div>
        </div>
      </div>
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
import GearIcon from '@/components/icons/Gear.vue'

const TOOL_NAME_FALLBACK = 'utility_textbox'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData } = useVueFlow()
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const runTraceRef = inject<Ref<Record<string, WarppStepTrace>>>('warppRunTrace', ref<Record<string, WarppStepTrace>>({}))
const runningRef = inject<Ref<boolean>>('warppRunning', ref(false))
const openResultModal = inject<(stepId: string, title: string) => void>('warppOpenResultModal', () => {})

const labelText = ref('')
const contentText = ref('')
const outputAttr = ref('')
const outputFrom = ref('')
const outputValue = ref('')
const showBack = ref(false)
const isDirty = ref(false)
const collapsed = ref(false)

const toolName = computed(() => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK)
const defaultAttributeHint = computed(() => `${props.id}_text`)
const headerLabel = computed(() => labelText.value.trim() || prettifyName(toolName.value))
const isDesignMode = computed(() => modeRef.value === 'design')
const runtimeTrace = computed(() => {
  const rec = runTraceRef.value
  if (!rec || typeof rec !== 'object') return undefined
  return rec[props.id]
})
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
const hasRuntimeDetails = computed(() => Boolean(runtimeTrace.value))

let suppressCommit = false

watch(
  () => props.data?.step,
  (step) => {
    suppressCommit = true
    const args = (step?.tool?.args ?? {}) as Record<string, unknown>
    labelText.value = String(args.label ?? step?.text ?? '')
    contentText.value = String(args.text ?? '')
    outputAttr.value = typeof args.output_attr === 'string' ? (args.output_attr as string) : ''
    outputFrom.value = typeof args.output_from === 'string' ? (args.output_from as string) : ''
    outputValue.value = typeof args.output_value === 'string' ? (args.output_value as string) : ''
    isDirty.value = false
    suppressCommit = false
  },
  { immediate: true, deep: true },
)

watch([labelText, contentText, outputAttr, outputFrom, outputValue], () => {
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
  const from = outputFrom.value.trim()
  const val = outputValue.value.trim()
  if (label) args.label = label
  if (text) args.text = text
  if (attr) args.output_attr = attr
  if (from) args.output_from = from
  if (val) args.output_value = val
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

function viewRuntimeDetails() {
  if (!runtimeTrace.value) return
  openResultModal(props.id, headerLabel.value)
}

function toggleBack(v?: boolean) {
  showBack.value = typeof v === 'boolean' ? v : !showBack.value
}

// Global expand/collapse signals injected from FlowView
const collapseAllSeq = inject<Ref<number>>('warppCollapseAllSeq', ref(0))
const expandAllSeq = inject<Ref<number>>('warppExpandAllSeq', ref(0))
const lastCollapseSeen = ref(0)
const lastExpandSeen = ref(0)
watch(collapseAllSeq, (v) => {
  if (typeof v === 'number' && v !== lastCollapseSeen.value) {
    lastCollapseSeen.value = v
    collapsed.value = true
  }
})
watch(expandAllSeq, (v) => {
  if (typeof v === 'number' && v !== lastExpandSeen.value) {
    lastExpandSeen.value = v
    collapsed.value = false
  }
})
</script>

<style scoped>
.flip-root { perspective: 800px; }
.flip-card { position: relative; transform-style: preserve-3d; transition: transform 200ms ease; }
.flip-card.is-flipped { transform: rotateX(180deg); }
.flip-face { backface-visibility: hidden; transform-style: preserve-3d; }
.flip-front { transform: rotateX(0deg); }
.flip-back { transform: rotateX(180deg); }
</style>

