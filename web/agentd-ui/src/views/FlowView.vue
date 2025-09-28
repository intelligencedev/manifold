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
        class="rounded bg-muted px-3 py-1 text-sm font-medium text-foreground transition hover:bg-muted/80"
        @click="onNew"
      >
        New
      </button>
      <button
        class="rounded bg-accent px-3 py-1 text-sm font-medium text-accent-foreground transition disabled:opacity-40"
        :disabled="!canSave"
        @click="onSave"
      >
        Save
      </button>
      <button
        class="rounded bg-primary px-3 py-1 text-sm font-medium text-primary-foreground transition disabled:opacity-40"
        :disabled="!canRun"
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
        class="rounded bg-muted px-3 py-1 text-sm font-medium text-foreground transition hover:bg-muted/80"
        @click="onCancelRun"
      >
        Cancel
      </button>
      <span v-if="loading" class="text-sm text-subtle-foreground">Loading…</span>
      <span v-else-if="error" class="text-sm text-danger-foreground">{{ error }}</span>
      <span v-else class="text-sm text-faint-foreground">Tools: {{ tools.length }}</span>
      <span v-if="runOutput" class="text-xs italic text-subtle-foreground truncate max-w-[320px]" :title="runOutput">Result: {{ runOutput }}</span>
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
            class="mt-3 max-h-[40vh] space-y-2 overflow-y-auto pr-1 lg:flex-1 lg:min-h-0 lg:max-h-none"
          >
            <div
              v-for="tool in tools"
              :key="tool.name"
              class="cursor-grab rounded border border-border/60 bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:border-accent hover:bg-surface"
              draggable="true"
              :title="tool.description ?? tool.name"
              @dragstart="(event: DragEvent) => onPaletteDragStart(event, tool)"
              @dragend="onPaletteDragEnd"
            >
              {{ tool.name }}
            </div>
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
            <Controls />
            <MiniMap />
          </VueFlow>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, provide, ref, watch } from 'vue'
import { VueFlow, type Edge, type Node, useVueFlow, type Connection } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'

import WarppStepNode from '@/components/flow/WarppStepNode.vue'
import {
  fetchWarppTools,
  fetchWarppWorkflow,
  fetchWarppWorkflows,
  saveWarppWorkflow,
  runWarppWorkflow,
} from '@/api/warpp'
import type { WarppStep, WarppTool, WarppWorkflow } from '@/types/warpp'
import type { StepNodeData } from '@/types/flow'

type LayoutMap = Record<string, { x: number; y: number }>

type StepNode = Node<StepNodeData> & { data: StepNodeData }

const DRAG_DATA_TYPE = 'application/warpp-tool'
const DEFAULT_LAYOUT_START_X = 140
const DEFAULT_LAYOUT_START_Y = 160
const DEFAULT_LAYOUT_HORIZONTAL_GAP = 320

const nodeTypes = { warppStep: WarppStepNode }

const { project } = useVueFlow()

const flowWrapper = ref<HTMLDivElement | null>(null)
const isDraggingFromPalette = ref(false)

const nodes = ref<StepNode[]>([])
const edges = ref<Edge[]>([])
const isHydrating = ref(false)

const workflowList = ref<WarppWorkflow[]>([])
const selectedIntent = ref<string>('')
const activeWorkflow = ref<WarppWorkflow | null>(null)

const tools = ref<WarppTool[]>([])
provide('warppTools', tools)
provide('warppHydrating', isHydrating)

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const running = ref(false)
const runOutput = ref('')
let runAbort: AbortController | null = null
const runLogs = ref<string[]>([])
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

const canSave = computed(() => !!activeWorkflow.value && !saving.value && dirty.value)
const canRun = computed(() => !!activeWorkflow.value && !saving.value && !running.value && nodes.value.length > 0)

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
  }
})

watch(selectedIntent, async (intent) => {
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
    return {
      id: step.id,
      type: 'warppStep',
      position,
      data: {
        order: idx,
        step: JSON.parse(JSON.stringify(step)) as WarppStep,
      },
      draggable: true,
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
  const id = generateStepId(tool.name)
  const order = nodes.value.length
  const step: WarppStep = {
    id,
    text: tool.description ?? tool.name,
    publish_result: false,
    tool: { name: tool.name },
  }

  const newNode: StepNode = {
    id,
    type: 'warppStep',
    position,
    data: {
      order,
      step,
    },
    draggable: true,
  }

  const updatedNodes = [...nodes.value, newNode]
  nodes.value = updatedNodes

  if (updatedNodes.length > 1) {
    const previous = updatedNodes[updatedNodes.length - 2]
    edges.value = [
      ...edges.value,
      { id: `e-${previous.id}-${newNode.id}`, source: previous.id, target: newNode.id },
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
    runLogs.value.push('✓ Run finished')
    if (runOutput.value) {
      runLogs.value.push('Result snippet: ' + runOutput.value.slice(0, 160) + (runOutput.value.length > 160 ? '…' : ''))
    }
    console.info('WARPP run summary:', runOutput.value)
  } catch (err: any) {
    if (err?.name === 'AbortError') {
      error.value = 'Run cancelled'
      runLogs.value.push('⚠ Run cancelled by user')
    } else {
      const msg = err?.message ?? 'Failed to run workflow'
      error.value = msg
      runLogs.value.push('✗ Error: ' + msg)
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

<style>
/* ensure flow canvas fills area */
.vue-flow__container {
  height: 100%;
}
</style>
