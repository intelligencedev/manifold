<template>
  <div class="flex h-full min-h-0 flex-col space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <label class="text-sm text-muted-foreground">
        Workflow
        <select
          v-model="selectedIntent"
          class="ml-2 rounded border border-border/70 bg-surface-muted/60 px-2 py-1 text-sm text-foreground"
        >
          <option disabled value="">Select workflow</option>
          <option v-for="wf in workflowList" :key="wf.intent" :value="wf.intent">
            {{ wf.intent }}
          </option>
        </select>
      </label>
      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-muted text-foreground hover:bg-muted/80 plain-link"
        title="Create new workflow"
        aria-label="New workflow"
        @click="onNew"
      >
        New
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-accent text-accent-foreground hover:bg-accent/90 plain-link"
        :disabled="!canSave"
        title="Save workflow"
        aria-label="Save workflow"
        @click="onSave"
      >
        Save
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-danger text-danger-foreground hover:bg-danger/90 plain-link"
        :disabled="!canDelete"
        title="Delete workflow"
        aria-label="Delete workflow"
        @click="onDelete"
      >
        Delete
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-muted text-foreground hover:bg-muted/80 plain-link"
        :disabled="!canExport"
        title="Export workflow as JSON"
        aria-label="Export workflow"
        @click="exportWorkflow"
      >
        Export
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-primary text-primary-foreground hover:bg-primary/90 plain-link"
        :disabled="!canRun"
        title="Run workflow"
        aria-label="Run workflow"
        @click="onRun"
      >
        <span v-if="!running">Run</span>
        <span v-else class="inline-flex items-center gap-1">
          <svg class="h-3 w-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle class="opacity-25" cx="12" cy="12" r="10" />
            <path class="opacity-75" d="M4 12a8 8 0 018-8" />
          </svg>
          Running…
        </span>
      </button>
      <button
        v-if="running"
        class="rounded bg-muted px-3 py-1 text-sm font-medium text-foreground transition hover:bg-muted/80 plain-link"
        @click="onCancelRun"
      >
        Cancel
      </button>
      <div class="flex flex-wrap items-center gap-3 ml-auto">
        <span v-if="loading" class="text-sm text-subtle-foreground">Loading…</span>
        <span v-else-if="error" class="text-sm text-danger-foreground">{{ error }}</span>
        <span v-else class="text-sm text-faint-foreground">Tools: {{ tools.length }}</span>
        <span
          v-if="runOutput"
          class="text-xs italic text-subtle-foreground truncate max-w-[320px]"
          :title="runOutput"
        >
          Result: {{ runOutput }}
        </span>
        <div class="flex items-center gap-1">
          <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Mode</span>
          <div class="inline-flex overflow-hidden rounded border border-border/60 text-xs">
            <button
              type="button"
              class="px-2 py-1 transition"
              :class="
                editorMode === 'design'
                  ? 'bg-accent text-accent-foreground'
                  : 'text-subtle-foreground hover:text-foreground'
              "
              @click="setEditorMode('design')"
            >
              Design
            </button>
            <button
              type="button"
              class="border-l border-border/60 px-2 py-1 transition disabled:opacity-40"
              :class="
                editorMode === 'run'
                  ? 'bg-accent text-accent-foreground'
                  : 'text-subtle-foreground hover:text-foreground'
              "
              :disabled="!hasRunTrace && !running"
              @click="setEditorMode('run')"
            >
              Run
            </button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="runLogs.length" class="max-h-32 overflow-y-auto rounded border border-border/50 bg-surface-muted px-3 py-2 text-xs font-mono leading-relaxed space-y-0.5">
      <div v-for="(l,i) in runLogs" :key="i" class="whitespace-pre-wrap break-words">{{ l }}</div>
    </div>

    <div
      class="flex flex-1 min-h-0 flex-col gap-4 overflow-auto lg:flex-row lg:items-stretch lg:overflow-hidden"
    >
      <aside class="lg:w-72">
        <div
          class="flex min-h-0 flex-col rounded-xl border border-border/70 bg-surface p-4 lg:h-full"
        >
          <div class="flex items-center justify-between gap-2">
            <h2 class="text-sm font-semibold text-foreground">Tool Palette</h2>
            <span class="text-[10px] uppercase tracking-wide text-faint-foreground"
              >Drag to add</span
            >
          </div>
          <p class="mt-1 text-xs text-subtle-foreground">
            Drag a tool onto the canvas to create a WARPP step.
          </p>

          <div
            class="mt-3 max-h-[40vh] space-y-3 overflow-y-auto pr-1 lg:flex-1 lg:min-h-0 lg:max-h-none"
          >
            <template v-if="utilityTools.length">
              <div class="space-y-2">
                <h3 class="text-[11px] font-semibold uppercase tracking-wide text-faint-foreground">
                  Utility Nodes
                </h3>
                <p class="text-[10px] text-subtle-foreground">
                  Utility nodes provide editor-only helpers for WARPP workflows.
                </p>
                <div
                  v-for="tool in utilityTools"
                  :key="tool.name"
                  class="cursor-grab rounded border border-border/60 bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:border-accent hover:bg-surface truncate"
                  draggable="true"
                  :title="tool.description ?? tool.name"
                  @dragstart="(event: DragEvent) => onPaletteDragStart(event, tool)"
                  @dragend="onPaletteDragEnd"
                >
                  {{ prettyUtilityLabel(tool.name) }}
                </div>
              </div>
            </template>
            <template v-if="workflowTools.length">
              <div class="space-y-2">
                <h3 class="text-[11px] font-semibold uppercase tracking-wide text-faint-foreground">
                  Workflow Tools
                </h3>
                <div
                  v-for="tool in workflowTools"
                  :key="tool.name"
                  class="cursor-grab rounded border border-border/60 bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:border-accent hover:bg-surface truncate"
                  draggable="true"
                  :title="tool.description ?? tool.name"
                  @dragstart="(event: DragEvent) => onPaletteDragStart(event, tool)"
                  @dragend="onPaletteDragEnd"
                >
                  {{ tool.name }}
                </div>
              </div>
            </template>
            <div
              v-if="!tools.length && !loading"
              class="rounded border border-dashed border-border/60 bg-surface-muted/60 p-3 text-xs text-subtle-foreground"
            >
              No tools available for this configuration.
            </div>
          </div>
        </div>
      </aside>

      <div class="flex-1 min-h-0">
        <div
          ref="flowWrapper"
          class="flex h-full min-h-0 w-full overflow-hidden rounded-xl border bg-surface"
          :class="isDraggingFromPalette ? 'border-accent/60' : 'border-border/70'"
        >
          <VueFlow
            v-model:nodes="nodes"
            v-model:edges="edges"
            :fit-view="true"
            :zoom-on-scroll="true"
            :zoom-on-double-click="false"
            :node-types="nodeTypes"
            class="h-full w-full"
            @dragover="onDragOver"
            @drop="onDrop"
            @connect="onConnect"
          >
            <Background />

            <!-- Themed Controls (replaces default Controls) -->
            <Panel position="bottom-left">
              <div
                class="flex items-center gap-1 rounded-md border border-border/70 bg-surface/90 p-1 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-surface/75"
              >
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Zoom in"
                  @click="onZoomIn"
                >
                  <ZoomInIcon class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Zoom out"
                  @click="onZoomOut"
                >
                  <ZoomOutIcon class="h-4 w-4" />
                </button>
                <span class="mx-0.5 h-5 w-px bg-border/60" aria-hidden="true"></span>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Fit view"
                  @click="onFitView"
                >
                  <FullScreenIcon class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  :aria-pressed="nodesLocked"
                  :aria-label="nodesLocked ? 'Unlock node positions' : 'Lock node positions'"
                  @click="toggleNodeLock"
                >
                  <UnlockedIcon v-if="!nodesLocked" class="h-4 w-4" />
                  <LockedIcon v-else class="h-4 w-4" />
                </button>
              </div>
            </Panel>

            <!-- Themed MiniMap -->
            <MiniMap
              v-if="showMiniMap"
              class="rounded-md border border-border/70 bg-surface/90 p-1 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-surface/75"
              :position="'bottom-right'"
              :pannable="true"
              :zoomable="true"
              :width="MINI_MAP_WIDTH"
              :height="MINI_MAP_HEIGHT"
              :mask-color="'rgb(var(--color-surface) / 0.85)'"
              :mask-stroke-color="'rgb(var(--color-border) / 0.7)'"
              :mask-stroke-width="1"
              :mask-border-radius="8"
              :node-color="miniMapNodeColor"
              :node-stroke-color="miniMapNodeStroke"
              :node-border-radius="6"
              :node-stroke-width="1"
            />

            <!-- Close button overlay for MiniMap (top-left of the MiniMap) -->
            <Panel
              v-if="showMiniMap"
              position="bottom-right"
              :style="{
                transform: `translate(calc(-${MINI_MAP_WIDTH}px + ${MINI_MAP_INSET}px), calc(-${MINI_MAP_HEIGHT}px + ${MINI_MAP_INSET}px))`,
              }"
            >
              <button
                type="button"
                class="inline-flex h-6 w-6 items-center justify-center rounded bg-surface text-subtle-foreground hover:text-foreground border border-border/60 shadow-sm"
                aria-label="Hide minimap"
                title="Hide minimap"
                @click="showMiniMap = false"
              >
                ×
              </button>
            </Panel>

            <!-- Collapsed show button when MiniMap hidden -->
            <Panel v-if="!showMiniMap" position="bottom-right">
              <button
                type="button"
                class="inline-flex items-center justify-center rounded-md border border-border/70 bg-surface/90 p-1.5 text-subtle-foreground shadow-sm backdrop-blur hover:text-foreground supports-[backdrop-filter]:bg-surface/75"
                aria-label="Show minimap"
                title="Show minimap"
                @click="showMiniMap = true"
              >
                <MapShowIcon class="h-5 w-5 -scale-x-100" />
              </button>
            </Panel>
          </VueFlow>
        </div>
      </div>
    </div>
    <div
      v-if="resultModal"
      class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8"
    >
      <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="closeResultModal"></div>
      <div
        class="relative z-10 flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl"
      >
        <div class="flex items-start justify-between gap-4 border-b border-border/60 px-6 py-4">
          <div class="space-y-1">
            <h3 class="text-base font-semibold text-foreground">{{ modalStepTitle }}</h3>
            <p v-if="modalStepId" class="text-xs text-subtle-foreground">ID: {{ modalStepId }}</p>
            <p v-if="modalStatusLabel" class="text-xs text-subtle-foreground">Status: {{ modalStatusLabel }}</p>
          </div>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-medium text-foreground hover:bg-surface-muted/80"
            @click="closeResultModal"
          >
            Close
          </button>
        </div>
        <div class="flex-1 overflow-y-auto px-6 py-4 text-sm text-foreground">
          <div v-if="activeModalTrace" class="space-y-5">
            <section v-if="activeModalTrace?.renderedArgs">
              <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">
                Rendered Arguments
              </h4>
              <pre
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.renderedArgs) }}</pre>
            </section>
            <section v-if="activeModalTrace?.delta">
              <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Delta</h4>
              <pre
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.delta) }}</pre>
            </section>
            <section v-if="activeModalTrace?.payload">
              <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Payload</h4>
              <pre
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.payload) }}</pre>
            </section>
            <p v-if="activeModalTrace?.error" class="rounded border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger-foreground">
              {{ activeModalTrace?.error }}
            </p>
          </div>
          <div v-else class="text-sm text-subtle-foreground">
            Trace data not yet available.
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, provide, ref, watch } from 'vue'
import { VueFlow, type Edge, type Node, useVueFlow, type Connection, Panel } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { MiniMap } from '@vue-flow/minimap'

import WarppStepNode from '@/components/flow/WarppStepNode.vue'
import WarppUtilityNode from '@/components/flow/WarppUtilityNode.vue'
import ZoomInIcon from '@/components/icons/ZoomIn.vue'
import ZoomOutIcon from '@/components/icons/ZoomOut.vue'
import FullScreenIcon from '@/components/icons/FullScreen.vue'
import LockedIcon from '@/components/icons/LockedBold.vue'
import UnlockedIcon from '@/components/icons/UnlockedBold.vue'
import MapShowIcon from '@/components/icons/MapShow.vue'
import {
  fetchWarppTools,
  fetchWarppWorkflow,
  fetchWarppWorkflows,
  saveWarppWorkflow,
  deleteWarppWorkflow,
  runWarppWorkflow,
} from '@/api/warpp'
import type { WarppStep, WarppTool, WarppWorkflow, WarppStepTrace } from '@/types/warpp'
import type { StepNodeData } from '@/types/flow'

type LayoutMap = Record<string, { x: number; y: number }>

type StepNode = Node<StepNodeData> & { data: StepNodeData }

const DRAG_DATA_TYPE = 'application/warpp-tool'
const DEFAULT_LAYOUT_START_X = 140
const DEFAULT_LAYOUT_START_Y = 160
const DEFAULT_LAYOUT_HORIZONTAL_GAP = 320
const UTILITY_TOOL_PREFIX = 'utility_'

const nodeTypes = { warppStep: WarppStepNode, warppUtility: WarppUtilityNode }

const { project, zoomIn, zoomOut, fitView, nodesDraggable } = useVueFlow()

const flowWrapper = ref<HTMLDivElement | null>(null)
const isDraggingFromPalette = ref(false)
// MiniMap visibility and sizing
const showMiniMap = ref(true)
const MINI_MAP_WIDTH = 180
const MINI_MAP_HEIGHT = 120
const MINI_MAP_INSET = 8

const nodes = ref<StepNode[]>([])
const edges = ref<Edge[]>([])
const isHydrating = ref(false)

const workflowList = ref<WarppWorkflow[]>([])
const selectedIntent = ref<string>('')
const activeWorkflow = ref<WarppWorkflow | null>(null)

const tools = ref<WarppTool[]>([])
provide('warppTools', tools)
provide('warppHydrating', isHydrating)
const editorMode = ref<'design' | 'run'>('design')
const runTrace = ref<Record<string, WarppStepTrace>>({})
provide('warppMode', editorMode)
provide('warppRunTrace', runTrace)

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const running = ref(false)
provide('warppRunning', running)
const runOutput = ref('')
let runAbort: AbortController | null = null
let runTraceTimers: ReturnType<typeof setTimeout>[] = []
const runLogs = ref<string[]>([])
const resultModal = ref<{ stepId: string; title: string } | null>(null)
const activeModalTrace = computed(() => {
  if (!resultModal.value) return undefined
  return runTrace.value[resultModal.value.stepId]
})
function openResultModal(stepId: string, title: string) {
  const hasTrace = runTrace.value[stepId]
  if (!hasTrace) return
  resultModal.value = { stepId, title }
}
function closeResultModal() {
  resultModal.value = null
}
provide('warppOpenResultModal', openResultModal)
provide('warppCloseResultModal', closeResultModal)
const dirty = ref(false)
// Track unsaved, locally-created workflows by intent
const localWorkflows = ref(new Map<string, WarppWorkflow>())

const toolMap = computed(() => {
  const map = new Map<string, WarppTool>()
  tools.value.forEach((tool) => {
    map.set(tool.name, tool)
  })
  return map
})

const workflowTools = computed(() => tools.value.filter((tool) => !isUtilityToolName(tool.name)))
const utilityTools = computed(() => tools.value.filter((tool) => isUtilityToolName(tool.name)))
const hasRunTrace = computed(() => Object.keys(runTrace.value).length > 0)
const modalStepTitle = computed(() => {
  if (!resultModal.value) return ''
  return resultModal.value.title || activeModalTrace.value?.text || resultModal.value.stepId
})
const modalStepId = computed(() => resultModal.value?.stepId ?? '')
const modalStatusLabel = computed(() => {
  const status = activeModalTrace.value?.status
  if (!status) return ''
  switch (status) {
    case 'completed':
      return 'Completed'
    case 'skipped':
      return 'Skipped'
    case 'noop':
      return 'Not executed'
    case 'error':
      return 'Error'
    default:
      return status
  }
})

const canSave = computed(() => !!activeWorkflow.value && !saving.value && dirty.value)
const canRun = computed(() => !!activeWorkflow.value && !saving.value && !running.value && nodes.value.length > 0)
const canExport = computed(() => !!activeWorkflow.value)
const canDelete = computed(() => !!activeWorkflow.value && !saving.value && !running.value)

// Node lock state: when true, nodes cannot be dragged
const nodesLocked = ref(false)
// Keep Vue Flow's global draggable flag in sync with our lock state
watch(
  nodesLocked,
  (locked) => {
    nodesDraggable.value = !locked
  },
  { immediate: true },
)

function onZoomIn() {
  zoomIn()
}
function onZoomOut() {
  zoomOut()
}
function onFitView() {
  fitView({ padding: 0.15 })
}
// schedule a fitView on next tick to ensure nodes/edges are rendered
async function scheduleFitView() {
  await nextTick()
  // small delay to let VueFlow compute bounds
  requestAnimationFrame(() => {
    try {
      fitView({ padding: 0.15 })
    } catch (e) {
      // ignore
    }
  })
}
function toggleNodeLock() {
  nodesLocked.value = !nodesLocked.value
}

function isUtilityToolName(name?: string | null): boolean {
  return typeof name === 'string' && name.startsWith(UTILITY_TOOL_PREFIX)
}

function prettyUtilityLabel(name: string): string {
  if (!isUtilityToolName(name)) return name
  const readable = name.slice(UTILITY_TOOL_PREFIX.length)
  return readable
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (ch) => ch.toUpperCase())
}

function clearRunTraceTimers() {
  runTraceTimers.forEach((id) => clearTimeout(id))
  runTraceTimers = []
}

function resetRunView() {
  clearRunTraceTimers()
  runTrace.value = {}
  editorMode.value = 'design'
  closeResultModal()
}

function applyRunTrace(entries: WarppStepTrace[]) {
  clearRunTraceTimers()
  runTrace.value = {}
  if (!entries.length) {
    return
  }
  entries.forEach((entry, index) => {
    const delay = Math.min(index * 150, 1500)
    const timer = setTimeout(() => {
      runTrace.value = { ...runTrace.value, [entry.stepId]: entry }
    }, delay)
    runTraceTimers.push(timer)
  })
}

function setEditorMode(mode: 'design' | 'run') {
  if (mode === editorMode.value) return
  if (mode === 'run' && !hasRunTrace.value && !running.value) {
    return
  }
  editorMode.value = mode
  if (mode === 'design') {
    closeResultModal()
  }
}

function formatJSON(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch (err) {
    console.warn('Failed to stringify value for modal', err)
    return String(value)
  }
}

// MiniMap styling helpers (use theme CSS variables)
function miniMapNodeColor() {
  // Base fill uses surface-muted for cohesion; selection handled by library styles
  return 'rgb(var(--color-surface-muted))'
}
function miniMapNodeStroke() {
  return 'rgb(var(--color-border))'
}

onMounted(async () => {
  loading.value = true
  try {
    const [toolResp, workflows] = await Promise.all([
      fetchWarppTools().catch((err) => {
        console.error('warpp tools', err)
        return [] as WarppTool[]
      }),
      fetchWarppWorkflows(),
    ])
    tools.value = toolResp
    workflowList.value = workflows
    if (selectedIntent.value) {
      await loadWorkflow(selectedIntent.value)
    } else if (workflows.length > 0) {
      selectedIntent.value = workflows[0].intent
    }
  } catch (err: any) {
    error.value = err?.message ?? 'Failed to load workflows'
  } finally {
    loading.value = false
    // initial fit once the initial load settles
    scheduleFitView()
  }
})

watch(selectedIntent, async (intent) => {
  resetRunView()
  if (!intent) {
    nodes.value = []
    edges.value = []
    activeWorkflow.value = null
    return
  }
  // If this is a locally-created unsaved workflow, hydrate from local instead of fetching
  const local = localWorkflows.value.get(intent)
  if (local) {
    error.value = ''
    isHydrating.value = true
    try {
      activeWorkflow.value = local
      nodes.value = []
      edges.value = []
      dirty.value = false
    } finally {
      await nextTick()
      isHydrating.value = false
    }
    return
  }
  loading.value = true
  error.value = ''
  try {
    await loadWorkflow(intent)
  } catch (err: any) {
    error.value = err?.message ?? 'Failed to load workflow'
  } finally {
    loading.value = false
  }
})

// Throttled sync to avoid heavy recomputation on each keystroke inside node editors
let syncScheduled = false
watch(
  nodes,
  () => {
    if (isHydrating.value) return
    if (syncScheduled) return
    syncScheduled = true
    requestAnimationFrame(() => {
      syncScheduled = false
      if (isHydrating.value) return
      syncWorkflowFromNodes()
      dirty.value = true
    })
  },
  { deep: true },
)

// Keep workflow.depends_on in sync when edges change (add/remove/reconnect)
watch(
  edges,
  () => {
    if (isHydrating.value) return
    if (syncScheduled) return
    syncScheduled = true
    requestAnimationFrame(() => {
      syncScheduled = false
      if (isHydrating.value) return
      syncWorkflowFromNodes()
      dirty.value = true
    })
  },
  { deep: true },
)

function workflowToNodes(wf: WarppWorkflow): StepNode[] {
  const layout = wf.ui?.layout ?? {}
  return wf.steps.map((step, idx) => {
    const stored = layout[step.id]
    const position = resolveNodePosition(stored, idx)
    const utility = isUtilityToolName(step.tool?.name)
    return {
      id: step.id,
      type: utility ? 'warppUtility' : 'warppStep',
      position,
      data: {
        order: idx,
        step: JSON.parse(JSON.stringify(step)) as WarppStep,
        kind: utility ? 'utility' : 'step',
      },
    }
  })
}

function resolveNodePosition(stored: { x: number; y: number } | undefined, index: number) {
  if (stored && Number.isFinite(stored.x) && Number.isFinite(stored.y)) {
    return { x: stored.x, y: stored.y }
  }
  return {
    x: DEFAULT_LAYOUT_START_X + index * DEFAULT_LAYOUT_HORIZONTAL_GAP,
    y: DEFAULT_LAYOUT_START_Y,
  }
}

function workflowToEdges(wf: WarppWorkflow): Edge[] {
  const out: Edge[] = []
  // Prefer explicit depends_on if present on any step
  const hasDag = wf.steps.some((s) => Array.isArray(s.depends_on) && s.depends_on.length > 0)
  if (hasDag) {
    for (const step of wf.steps) {
      const deps = step.depends_on ?? []
      for (const dep of deps) {
        out.push({ id: `e-${dep}-${step.id}`, source: dep, target: step.id })
      }
    }
    return out
  }
  // Fallback to sequential
  for (let i = 1; i < wf.steps.length; i += 1) {
    const prev = wf.steps[i - 1]
    const curr = wf.steps[i]
    out.push({ id: `e-${prev.id}-${curr.id}`, source: prev.id, target: curr.id })
  }
  return out
}

async function loadWorkflow(intent: string) {
  isHydrating.value = true
  try {
    const wf = await fetchWarppWorkflow(intent)
    const nextNodes = workflowToNodes(wf)
    const nextEdges = workflowToEdges(wf)

    activeWorkflow.value = wf
    edges.value = nextEdges
    nodes.value = nextNodes
  } finally {
    await nextTick()
    isHydrating.value = false
    // Fit view after nodes are rendered
    scheduleFitView()
  }
}

function syncWorkflowFromNodes() {
  if (isHydrating.value) {
    return
  }
  if (!activeWorkflow.value) {
    return
  }
  const orderedNodes = [...nodes.value].sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
  // Build depends_on from current edges graph
  const incoming: Record<string, string[]> = {}
  for (const e of edges.value) {
    if (!incoming[e.target]) incoming[e.target] = []
    incoming[e.target].push(e.source)
  }
  const steps = orderedNodes.map((node) => {
    const step = { ...(node.data?.step ?? ({} as WarppStep)) }
    step.id = node.id
    step.depends_on = (incoming[node.id] ?? []).slice()
    return step
  })
  activeWorkflow.value = { ...activeWorkflow.value, steps }
}

function onDragOver(event: DragEvent) {
  if (!event.dataTransfer?.types.includes(DRAG_DATA_TYPE)) {
    return
  }
  event.preventDefault()
  event.dataTransfer.dropEffect = 'copy'
}

function onDrop(event: DragEvent) {
  if (!event.dataTransfer?.types.includes(DRAG_DATA_TYPE)) {
    return
  }
  event.preventDefault()
  isDraggingFromPalette.value = false

  const toolName = event.dataTransfer.getData(DRAG_DATA_TYPE)
  if (!toolName) {
    return
  }
  const tool = toolMap.value.get(toolName)
  if (!tool || !flowWrapper.value) {
    return
  }

  const bounds = flowWrapper.value.getBoundingClientRect()
  const position = project({
    x: event.clientX - bounds.left,
    y: event.clientY - bounds.top,
  })

  addToolNode(tool, position)
}

function onPaletteDragStart(event: DragEvent, tool: WarppTool) {
  if (!event.dataTransfer) {
    return
  }
  isDraggingFromPalette.value = true
  event.dataTransfer.setData(DRAG_DATA_TYPE, tool.name)
  event.dataTransfer.setData('text/plain', tool.name)
  event.dataTransfer.effectAllowed = 'copyMove'
}

function onPaletteDragEnd() {
  isDraggingFromPalette.value = false
}

function onConnect(connection: Connection) {
  const { source, target } = connection
  if (!source || !target) return
  if (source === target) return // no self-loop
  // prevent duplicate edges
  if (edges.value.some((e) => e.source === source && e.target === target)) return
  const id = `e-${source}-${target}-${Math.random().toString(36).slice(2, 8)}`
  edges.value = [...edges.value, { id, source, target }]
}

function addToolNode(tool: WarppTool, position: { x: number; y: number }) {
  if (!activeWorkflow.value) {
    return
  }
  if (isUtilityToolName(tool.name)) {
    appendNode(createUtilityNode(tool, position))
    return
  }
  appendNode(createWorkflowNode(tool, position))
}

function createWorkflowNode(tool: WarppTool, position: { x: number; y: number }): StepNode {
  const id = generateStepId(tool.name)
  const order = nodes.value.length
  const step: WarppStep = {
    id,
    text: tool.description ?? tool.name,
    publish_result: false,
    tool: { name: tool.name },
  }
  return {
    id,
    type: 'warppStep',
    position,
    data: {
      order,
      step,
      kind: 'step',
    },
  }
}

function createUtilityNode(tool: WarppTool, position: { x: number; y: number }): StepNode {
  const id = generateStepId(tool.name)
  const order = nodes.value.length
  const displayName = prettyUtilityLabel(tool.name)
  const step: WarppStep = {
    id,
    text: displayName,
    publish_result: false,
    tool: { name: tool.name, args: { label: displayName, text: '', output_attr: '' } },
  }
  return {
    id,
    type: 'warppUtility',
    position,
    data: {
      order,
      step,
      kind: 'utility',
    },
  }
}

function appendNode(node: StepNode) {
  const updatedNodes = [...nodes.value, node]
  nodes.value = updatedNodes
  if (updatedNodes.length > 1) {
    const previous = updatedNodes[updatedNodes.length - 2]
    edges.value = [
      ...edges.value,
      { id: `e-${previous.id}-${node.id}`, source: previous.id, target: node.id },
    ]
  }
}

function generateStepId(toolName: string): string {
  const base =
    toolName
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/(^-|-$)/g, '') || 'step'
  let candidate = ''
  do {
    const unique =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? (crypto.randomUUID?.() ?? Math.random().toString(36).slice(2, 10))
        : Math.random().toString(36).slice(2, 10)
    candidate = `${base}-${unique.slice(0, 8)}`
  } while (nodes.value.some((node) => node.id === candidate))
  return candidate
}

async function onSave(): Promise<WarppWorkflow | null> {
  if (!activeWorkflow.value) return null
  if (!dirty.value) return activeWorkflow.value
  saving.value = true
  error.value = ''
  try {
    const orderedNodes = [...nodes.value].sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
    const incoming: Record<string, string[]> = {}
    for (const e of edges.value) {
      if (!incoming[e.target]) incoming[e.target] = []
      incoming[e.target].push(e.source)
    }
    const steps = orderedNodes.map((node) => {
      const step = { ...(node.data?.step ?? ({} as WarppStep)) }
      step.id = node.id
      step.depends_on = (incoming[node.id] ?? []).slice()
      return step as WarppStep
    })
    const layout: LayoutMap = {}
    orderedNodes.forEach((node) => {
      const pos = node.position ?? { x: 0, y: 0 }
      layout[node.id] = { x: pos.x, y: pos.y }
    })
    const payload: WarppWorkflow = {
      ...activeWorkflow.value,
      steps,
      ui: { ...(activeWorkflow.value.ui ?? {}), layout },
    }
    runLogs.value.push('[save] PUT /api/warpp/workflows/' + encodeURIComponent(payload.intent))
    const saved = await saveWarppWorkflow(payload)
    runLogs.value.push('[save] 200 OK')
    // If this workflow was locally-created, clear the local marker
    localWorkflows.value.delete(payload.intent)
    const listIdx = workflowList.value.findIndex((wf) => wf.intent === saved.intent)
    if (listIdx !== -1) workflowList.value.splice(listIdx, 1, saved)
    else workflowList.value.push(saved)
    isHydrating.value = true
    try {
      activeWorkflow.value = saved
      nodes.value = workflowToNodes(saved)
      edges.value = workflowToEdges(saved)
      dirty.value = false
    } finally {
      await nextTick()
      isHydrating.value = false
    }
    // Fit after successful save rehydrate
    scheduleFitView()
    return saved
  } catch (err: any) {
    const msg = err?.message ?? 'Failed to save workflow'
    error.value = msg
    runLogs.value.push('[save] error: ' + msg)
    return null
  } finally {
    saving.value = false
  }
}

async function onRun() {
  if (!activeWorkflow.value) return
  running.value = true
  error.value = ''
  runOutput.value = ''
  runLogs.value = []
  runAbort?.abort()
  runAbort = new AbortController()
  editorMode.value = 'run'
  clearRunTraceTimers()
  runTrace.value = {}
  const intent = activeWorkflow.value.intent
  runLogs.value.push(`▶ Starting run for intent "${intent}"`)
  // Capture need to save at start (canSave may change mid-process)
  const needSave = canSave.value
  if (needSave) {
    runLogs.value.push('… Saving workflow before run')
    const saved = await onSave()
    if (saved) runLogs.value.push('✓ Save complete')
    else runLogs.value.push('✗ Save failed – proceeding with current in-memory workflow')
  }
  try {
    runLogs.value.push('→ POST /api/warpp/run')
    console.debug('[warpp] POST /api/warpp/run intent=%s', intent)
    ;(window as any).__warppLastRunRequest = { intent, ts: Date.now() }
    const res = await runWarppWorkflow(intent, `Run workflow: ${intent}`, runAbort.signal)
    runOutput.value = res.result || ''
    applyRunTrace(res.trace ?? [])
    runLogs.value.push('✓ Run finished')
    if (runOutput.value) {
      runLogs.value.push('Result snippet: ' + runOutput.value.slice(0, 160) + (runOutput.value.length > 160 ? '…' : ''))
    }
    console.info('WARPP run summary:', runOutput.value)
  } catch (err: any) {
    if (err?.name === 'AbortError') {
      error.value = 'Run cancelled'
      runLogs.value.push('⚠ Run cancelled by user')
      resetRunView()
    } else {
      const msg = err?.message ?? 'Failed to run workflow'
      error.value = msg
      runLogs.value.push('✗ Error: ' + msg)
      resetRunView()
    }
  } finally {
    running.value = false
  }
}

function onCancelRun() {
  if (running.value && runAbort) {
    runAbort.abort()
  }
}

async function onDelete() {
  if (!activeWorkflow.value) return
  const intent = activeWorkflow.value.intent
  const confirmed = window.confirm(`Delete workflow "${intent}"? This cannot be undone.`)
  if (!confirmed) return
  try {
    await deleteWarppWorkflow(intent)
    // Remove from local list/maps and reset selection
    localWorkflows.value.delete(intent)
    const idx = workflowList.value.findIndex(w => w.intent === intent)
    if (idx !== -1) workflowList.value.splice(idx, 1)
    if (selectedIntent.value === intent) {
      selectedIntent.value = workflowList.value[0]?.intent ?? ''
    }
    activeWorkflow.value = null
    nodes.value = []
    edges.value = []
    dirty.value = false
  } catch (err: any) {
    alert(err?.message ?? 'Failed to delete workflow')
  }
}

function exportWorkflow() {
  if (!activeWorkflow.value) return
  // Build latest payload mirroring save logic (without network)
  const orderedNodes = [...nodes.value].sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
  const incoming: Record<string, string[]> = {}
  for (const e of edges.value) {
    if (!incoming[e.target]) incoming[e.target] = []
    incoming[e.target].push(e.source)
  }
  const steps = orderedNodes.map((node) => {
    const step = { ...(node.data?.step ?? ({} as WarppStep)) }
    step.id = node.id
    step.depends_on = (incoming[node.id] ?? []).slice()
    return step as WarppStep
  })
  const layout: LayoutMap = {}
  orderedNodes.forEach((node) => {
    const pos = node.position ?? { x: 0, y: 0 }
    layout[node.id] = { x: pos.x, y: pos.y }
  })
  const payload: WarppWorkflow = {
    ...activeWorkflow.value,
    steps,
    ui: { ...(activeWorkflow.value.ui ?? {}), layout },
  }

  // Safe stringify with cycle protection and function stripping
  const seen = new WeakSet()
  const json = JSON.stringify(
    payload,
    (_k, val) => {
      if (typeof val === 'function' || typeof val === 'symbol') return undefined
      if (val && typeof val === 'object') {
        if (seen.has(val)) return undefined
        seen.add(val)
      }
      return val
    },
    2,
  )

  const ts = new Date().toISOString().replace(/[:]/g, '-')
  // payload may come from older formats that used `name`; assert to any to access safely
  const base = (payload.intent || (payload as any).name || 'workflow')
  const filename = `${base}-${ts}.json`

  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

function normalizeIntent(input: string): string {
  // Conservative normalization: trim and collapse spaces, restrict to [a-z0-9._-]
  // Keep it readable and filesystem-friendly
  const t = input.trim().toLowerCase()
  const collapsed = t.replace(/\s+/g, '-')
  const safe = collapsed.replace(/[^a-z0-9._-]/g, '-')
  return safe.replace(/^-+|-+$/g, '').slice(0, 64) || 'workflow'
}

async function onNew() {
  const name = window.prompt('Enter a name for the new workflow (intent):', '')
  if (name === null) return
  const intent = normalizeIntent(name)
  if (!intent) {
    alert('Please enter a valid name')
    return
  }
  if (workflowList.value.some((w) => w.intent === intent) || localWorkflows.value.has(intent)) {
    alert('A workflow with that name already exists')
    return
  }
  const wf: WarppWorkflow = { intent, description: '', steps: [] }
  // Track locally and show in dropdown immediately
  localWorkflows.value.set(intent, wf)
  workflowList.value.push(wf)
  // Switch to the new workflow view
  isHydrating.value = true
  try {
    selectedIntent.value = intent
    activeWorkflow.value = wf
    nodes.value = []
    edges.value = []
    dirty.value = false
  } finally {
    await nextTick()
    isHydrating.value = false
  }
}
</script>

<style scoped>
/* Make workflow header buttons appear as plain text links with underline on hover */
.plain-link {
  background: none !important;
  border: none !important;
  padding: 0 !important;
  color: inherit !important;
  font: inherit !important;
  text-decoration: none !important;
  cursor: pointer !important;
}
.plain-link:hover {
  text-decoration: underline !important;
  background: none !important;
}
</style>

<style>
/* ensure flow canvas fills area */
.vue-flow__container {
  height: 100%;
}
</style>
