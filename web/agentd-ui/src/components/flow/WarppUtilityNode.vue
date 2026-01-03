<template>
  <WarppBaseNode
    :collapsed="collapsed"
    :min-width="collapsed ? WARPP_UTILITY_NODE_COLLAPSED.width : UTILITY_MIN_WIDTH"
    :min-height="collapsed ? WARPP_UTILITY_NODE_COLLAPSED.height : UTILITY_MIN_HEIGHT"
    :min-width-px="nodeMinWidthPx"
    :min-height-px="nodeMinHeightPx"
    :show-resizer="isDesignMode"
    :show-back="showBack"
    :root-class="rootClass"
    :selected="props.selected"
    @resize-end="onResizeEnd"
  >
    <template #front>
      <!-- Header with gear -->
      <div class="flex items-start justify-between gap-2">
        <div class="flex-1">
          <div class="flex items-center gap-2">
            <button
              class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
              :aria-expanded="!collapsed"
              :title="collapsed ? 'Expand' : 'Collapse'"
              @click.prevent.stop="toggleCollapsed()"
            >
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
        <div class="flex items-center gap-1">
          <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Utility</span>
          <button
            class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
            title="Advanced (promote to attribute)"
            aria-label="Advanced (promote to attribute)"
            @click.prevent.stop="toggleBack(true)"
          >
            <GearIcon class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      <!-- Node ID chip row (below header, always visible) -->
      <div class="mt-1 flex items-center justify-between gap-2">
        <button
          class="hidden sm:inline-flex max-w-[200px] items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono text-foreground/80 hover:bg-muted/60"
          :title="copied ? 'Copied!' : `Copy step id: ${props.id}`"
          @click.prevent.stop="copyStepId"
        >
          <svg v-if="!copied" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
          </svg>
          <svg v-else xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="h-3.5 w-3.5 text-green-500" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
          <span class="truncate">{{ props.id }}</span>
        </button>
      </div>

      <!-- Front content -->
      <div class="mt-3 warpp-utility-content flex flex-col h-full" :class="collapsed ? 'hidden' : ''">
        <div class="flex-1 space-y-2 overflow-auto">
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
            <div class="flex items-center justify-between gap-2">
              <span>{{ isAgentResponse ? 'Response Content' : 'Textbox Content' }}</span>
              <div
                v-if="isAgentResponse"
                class="flex items-center gap-1 text-[10px] uppercase tracking-wide text-faint-foreground"
              >
                <span>Render</span>
                <DropdownSelect
                  v-model="renderMode"
                  size="xs"
                  class="text-[11px]"
                  :disabled="!isDesignMode"
                  :options="[
                    { id: 'raw', label: 'Raw text', value: 'raw' },
                    { id: 'markdown', label: 'Markdown', value: 'markdown' },
                    { id: 'html', label: 'HTML', value: 'html' },
                  ]"
                  @update:modelValue="markDirty"
                />
              </div>
            </div>
            <textarea
              v-if="isDesignMode"
              v-model="contentText"
              rows="4"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto w-full flex-1 min-h-[92px] resize-none whitespace-pre-wrap break-words"
              placeholder="Enter static text or use ${A.key} placeholders"
              @input="markDirty"
              @wheel.stop
            ></textarea>
            <div
              v-else
              :class="[
                'flex-1',
                'rounded border border-border/60 bg-surface-muted px-2 py-2 text-[11px] text-foreground overflow-auto w-full',
              ]"
              style="contain: content; overflow-wrap: anywhere;"
              @wheel.stop
            >
              <template v-if="isAgentResponse">
                <div v-if="hasDisplayText" class="h-full overflow-auto pr-1 space-y-1">
                  <pre
                    v-if="renderMode === 'raw'"
                    class="whitespace-pre-wrap break-words text-[11px] leading-relaxed text-foreground"
                  >{{ displayText }}</pre>
                  <div
                    v-else
                    class="agent-response-render text-[11px] leading-relaxed space-y-1 break-words"
                    v-html="renderedContent"
                  ></div>
                </div>
                <span v-else class="block">
                  {{ runtimeStatus === 'pending' ? 'Waiting for execution…' : 'Run the workflow to see rendered text.' }}
                </span>
              </template>
              <template v-else>
                <span class="block">{{ runtimeText || 'Run the workflow to see resolved text.' }}</span>
              </template>
            </div>
          </label>
          <div
            v-if="isAgentResponse && isDesignMode"
            class="rounded border border-border/60 bg-surface-muted/70 px-2 py-2 text-[11px] text-foreground"
          >
            <div class="flex items-center justify-between text-[10px] uppercase tracking-wide text-faint-foreground">
              <span>Preview</span>
              <span class="text-[10px] capitalize">{{ renderMode }}</span>
            </div>
            <div class="mt-1 max-h-[140px] overflow-auto pr-1 space-y-1">
              <template v-if="hasDisplayText">
                <pre
                  v-if="renderMode === 'raw'"
                  class="whitespace-pre-wrap break-words text-[11px] leading-relaxed text-foreground"
                >{{ displayText }}</pre>
                <div
                  v-else
                  class="agent-response-render text-[11px] leading-relaxed space-y-1 break-words"
                  v-html="renderedContent"
                ></div>
              </template>
              <p v-else class="text-[10px] text-faint-foreground">Add content to see a live preview.</p>
            </div>
          </div>
          <p v-if="runtimeStatus === 'pending'" class="text-[10px] italic text-faint-foreground">
            Waiting for execution…
          </p>
          <p v-else-if="runtimeStatusMessage" class="text-[10px] italic text-faint-foreground">
            {{ runtimeStatusMessage }}
          </p>
          <p v-if="runtimeError && runtimeStatus !== 'pending'" class="rounded border border-danger/40 bg-danger/10 px-2 py-1 text-[10px] text-danger-foreground">
            <span class="block max-h-[6rem] overflow-auto whitespace-pre-wrap break-words" @wheel.stop>{{ runtimeError }}</span>
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
    </template>

    <template #back>
      <div class="flex items-start justify-between gap-2">
        <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Advanced • Promote to attribute (optional)</span>
        <button
          class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
          title="Back"
          aria-label="Back"
          @click.prevent.stop="toggleBack(false)"
        >
          <GearIcon class="h-3.5 w-3.5" />
        </button>
      </div>
      <div class="mt-3" :class="collapsed ? 'hidden' : ''">
        <div class="space-y-2">
          <p class="text-[10px] text-faint-foreground">
            Prefer referencing prior step data with
            <code>{{ `\${A.${props.id}.json...}` }}</code>.
            Promote to an attribute when you want a short, stable name (useful for guards and reuse).
          </p>
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
              placeholder="payload | json.<path> | delta.<key> | args.<key>"
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
    </template>
  </WarppBaseNode>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, onMounted, type CSSProperties } from 'vue'
import { useVueFlow, type NodeProps } from '@vue-flow/core'
import type { OnResizeEnd } from '@vue-flow/node-resizer'

import WarppBaseNode from './WarppBaseNode.vue'
import type { StepNodeData } from '@/types/flow'
import type { WarppStep, WarppStepTrace } from '@/types/warpp'
import type { Ref } from 'vue'
import GearIcon from '@/components/icons/Gear.vue'
import { WARPP_UTILITY_NODE_DIMENSIONS, WARPP_UTILITY_NODE_COLLAPSED } from '@/constants/warppNodes'
import { renderMarkdown } from '@/utils/markdown'
import DropdownSelect from '@/components/DropdownSelect.vue'

const TOOL_NAME_FALLBACK = 'utility_textbox'
const AGENT_RESPONSE_TOOL = 'agent_response'
type RenderMode = 'raw' | 'markdown' | 'html'

const props = defineProps<NodeProps<StepNodeData>>()

const { updateNodeData, updateNode } = useVueFlow()

const UTILITY_MIN_WIDTH = WARPP_UTILITY_NODE_DIMENSIONS.minWidth
const UTILITY_MIN_HEIGHT = WARPP_UTILITY_NODE_DIMENSIONS.minHeight
const nodeMinWidthPx = computed(() => (collapsed.value ? `${WARPP_UTILITY_NODE_COLLAPSED.width}px` : `${UTILITY_MIN_WIDTH}px`))
const nodeMinHeightPx = computed(() => (collapsed.value ? `${WARPP_UTILITY_NODE_COLLAPSED.height}px` : `${UTILITY_MIN_HEIGHT}px`))
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const runTraceRef = inject<Ref<Record<string, WarppStepTrace>>>('warppRunTrace', ref<Record<string, WarppStepTrace>>({}))
const runningRef = inject<Ref<boolean>>('warppRunning', ref(false))
const openResultModal = inject<(stepId: string, title: string) => void>('warppOpenResultModal', () => {})

function normalizeDisplayText(value: unknown): string {
  if (value === null || value === undefined) return ''
  const str = typeof value === 'string' ? value : String(value)
  // Replace escaped newlines so literal "\n" render as actual line breaks
  return str.replace(/\r\n/g, '\n').replace(/\\n/g, '\n')
}

function parseRenderMode(value: unknown): RenderMode {
  const mode = typeof value === 'string' ? (value as RenderMode) : 'markdown'
  return mode === 'raw' || mode === 'html' || mode === 'markdown' ? mode : 'markdown'
}

const labelText = ref('')
const contentText = ref('')
const renderMode = ref<RenderMode>('markdown')
const outputAttr = ref('')
const outputFrom = ref('')
const outputValue = ref('')
const showBack = ref(false)
const rootClass = computed(() => [
  collapsed.value ? 'min-w-[220px] min-h-[72px]' : 'min-w-[320px] min-h-[200px] h-full',
  'transition-colors duration-150 ease-out',
])
const isDirty = ref(false)
const collapsed = ref(true)
const copied = ref(false)

const toolName = computed(() => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK)
const isAgentResponse = computed(() => toolName.value === AGENT_RESPONSE_TOOL)
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
  if (typeof text !== 'string') return ''
  return isAgentResponse.value ? normalizeDisplayText(text) : text
})
const designText = computed(() => (isAgentResponse.value ? normalizeDisplayText(contentText.value) : contentText.value))
const displayText = computed(() => (isDesignMode.value ? designText.value : runtimeText.value))
const hasDisplayText = computed(() => displayText.value.length > 0)
const renderedContent = computed(() => {
  const text = displayText.value
  if (!text) return ''
  switch (renderMode.value) {
    case 'markdown':
      return renderMarkdown(text)
    case 'html':
      return text.replace(/\n/g, '<br />')
    case 'raw':
    default:
      return text
  }
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
    renderMode.value = parseRenderMode(args.render_mode)
    outputAttr.value = typeof args.output_attr === 'string' ? (args.output_attr as string) : ''
    outputFrom.value = typeof args.output_from === 'string' ? (args.output_from as string) : ''
    outputValue.value = typeof args.output_value === 'string' ? (args.output_value as string) : ''
    isDirty.value = false
    suppressCommit = false
  },
  { immediate: true, deep: true },
)

watch([labelText, contentText, outputAttr, outputFrom, outputValue, renderMode], () => {
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
  if (isAgentResponse.value) args.render_mode = renderMode.value
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
  if (name === AGENT_RESPONSE_TOOL) return 'Agent Response'
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

// Remember last expanded size per node so we can restore on expand
const prevExpandedSize = new Map<string, { w: number; h: number }>()

function px(n: number) {
  return `${Math.round(n)}px`
}

function applyCollapsedStyle(next: boolean) {
  const nodeId = props.id
  updateNode(nodeId, (node) => {
    const baseStyle: CSSProperties =
      typeof node.style === 'function' ? ((node.style(node) as CSSProperties) ?? {}) : { ...(node.style ?? {}) }

    if (next) {
      // store current explicit size to restore later
      const currW = typeof (baseStyle as any).width === 'string' ? parseFloat((baseStyle as any).width as string) : undefined
      const currH = typeof (baseStyle as any).height === 'string' ? parseFloat((baseStyle as any).height as string) : undefined
      if (currW && currH) prevExpandedSize.set(nodeId, { w: currW, h: currH })
      return {
        style: {
          ...baseStyle,
          width: px(WARPP_UTILITY_NODE_COLLAPSED.width),
          height: px(WARPP_UTILITY_NODE_COLLAPSED.height),
          minWidth: px(WARPP_UTILITY_NODE_COLLAPSED.width),
          minHeight: px(WARPP_UTILITY_NODE_COLLAPSED.height),
        },
      }
    }

    // expanding: try to restore previous explicit size, else defaults
    const restored = prevExpandedSize.get(nodeId)
    const targetW = restored?.w ?? WARPP_UTILITY_NODE_DIMENSIONS.defaultWidth
    const targetH = restored?.h ?? WARPP_UTILITY_NODE_DIMENSIONS.defaultHeight
    return {
      style: {
        ...baseStyle,
        width: px(targetW),
        height: px(targetH),
        minWidth: px(WARPP_UTILITY_NODE_DIMENSIONS.minWidth),
        minHeight: px(WARPP_UTILITY_NODE_DIMENSIONS.minHeight),
      },
    }
  })
  collapsed.value = next
  const nextData = { ...(props.data ?? { kind: 'utility', order: 0 } as any), collapsed: next }
  updateNodeData(props.id, nextData)
}

function toggleCollapsed(v?: boolean) {
  applyCollapsedStyle(typeof v === 'boolean' ? v : !collapsed.value)
}

async function copyStepId() {
  try {
    await navigator.clipboard.writeText(props.id)
    copied.value = true
    setTimeout(() => (copied.value = false), 1200)
  } catch (err) {
    window.prompt('Copy step id', props.id)
  }
}

// Global expand/collapse signals injected from FlowView
const collapseAllSeq = inject<Ref<number>>('warppCollapseAllSeq', ref(0))
const expandAllSeq = inject<Ref<number>>('warppExpandAllSeq', ref(0))
const lastCollapseSeen = ref(0)
const lastExpandSeen = ref(0)
watch(collapseAllSeq, (v) => {
  if (typeof v === 'number' && v !== lastCollapseSeen.value) {
    lastCollapseSeen.value = v
    applyCollapsedStyle(true)
  }
})

function onResizeEnd(event: OnResizeEnd) {
  if (!isDesignMode.value) return
  const widthPx = `${Math.round(event.params.width)}px`
  const heightPx = `${Math.round(event.params.height)}px`
    updateNode(props.id, (node) => {
      const baseStyle: CSSProperties =
        typeof node.style === 'function' ? (node.style(node) as CSSProperties) ?? {} : { ...(node.style ?? {}) }
      return {
        style: {
          ...baseStyle,
          width: widthPx,
          height: heightPx,
          minWidth: px(UTILITY_MIN_WIDTH),
          minHeight: px(UTILITY_MIN_HEIGHT),
        },
      }
    })
  isDirty.value = true
}
watch(expandAllSeq, (v) => {
  if (typeof v === 'number' && v !== lastExpandSeen.value) {
    lastExpandSeen.value = v
    applyCollapsedStyle(false)
  }
})

// Sync with externally provided ui flag if present
watch(
  () => (props.data as any)?.collapsed,
  (next) => {
    if (typeof next === 'boolean') {
      applyCollapsedStyle(next)
    }
  },
  { immediate: false },
)

// Apply initial collapsed dimensions so hitbox matches visual state
onMounted(() => {
  const initial = typeof (props.data as any)?.collapsed === 'boolean' ? (props.data as any).collapsed : true
  applyCollapsedStyle(initial)
})
</script>

<style scoped>
.agent-response-render :deep(p) {
  margin: 0.15rem 0;
}

.agent-response-render :deep(ul),
.agent-response-render :deep(ol) {
  padding-left: 1.05rem;
  margin: 0.25rem 0;
}

.agent-response-render :deep(pre) {
  background-color: rgb(var(--color-surface-muted));
  border-radius: 6px;
  padding: 0.5rem;
  overflow-x: auto;
}
</style>
