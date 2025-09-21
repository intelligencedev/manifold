<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <label class="text-sm text-slate-300">
        Workflow
        <select
          v-model="selectedIntent"
          class="ml-2 rounded border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-white"
        >
          <option disabled value="">Select workflow</option>
          <option v-for="wf in workflowList" :key="wf.intent" :value="wf.intent">
            {{ wf.intent }}
          </option>
        </select>
      </label>
      <button
        class="rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
        :disabled="!canSave"
        @click="onSave"
      >
        Save
      </button>
      <span v-if="loading" class="text-sm text-slate-400">Loading…</span>
      <span v-else-if="error" class="text-sm text-red-400">{{ error }}</span>
      <span v-else class="text-sm text-slate-500">
        Tools: {{ tools.length }}
      </span>
    </div>

    <div class="h-[60vh] rounded-xl border border-slate-800 overflow-hidden">
      <VueFlow
        v-model:nodes="nodes"
        v-model:edges="edges"
        :fit-view="true"
        :zoom-on-scroll="true"
        :zoom-on-double-click="false"
        class="bg-slate-900/60"
        @node-click="onNodeClick"
        @pane-click="onPaneClick"
      >
        <Background />
        <Controls />
        <MiniMap />
      </VueFlow>
    </div>

    <div
      v-if="selectedNode"
      class="space-y-4 rounded-xl border border-slate-800 bg-slate-900/60 p-4"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
  <h3 class="text-base font-semibold text-white">Step {{ selectedNode?.id }}</h3>
  <span class="text-xs uppercase tracking-wide text-slate-400">Order {{ (selectedNode?.data?.order ?? 0) + 1 }}</span>
      </div>
      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-1 text-sm text-slate-300">
          Text
          <input
            v-model="stepForm.text"
            type="text"
            class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-white"
            placeholder="Describe the step"
          />
        </label>
        <label class="flex flex-col gap-1 text-sm text-slate-300">
          Guard
          <input
            v-model="stepForm.guard"
            type="text"
            class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-white"
            placeholder="Example: A.os != 'windows'"
          />
        </label>
        <label class="flex items-center gap-2 text-sm text-slate-300">
          <input v-model="stepForm.publishResult" type="checkbox" />
          Publish result
        </label>
        <label class="flex flex-col gap-1 text-sm text-slate-300">
          Tool
          <select
            v-model="stepForm.toolName"
            class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-white"
          >
            <option value="">(none)</option>
            <option v-for="tool in tools" :key="tool.name" :value="tool.name">
              {{ tool.name }}
            </option>
          </select>
        </label>
      </div>
      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-1 text-sm text-slate-300">
          Tool Args (JSON)
          <textarea
            v-model="stepForm.toolArgsText"
            rows="8"
            class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-white font-mono"
            placeholder="{ &quot;query&quot;: &quot;${A.query}&quot; }"
          ></textarea>
          <span v-if="toolArgsError" class="text-xs text-red-400">{{ toolArgsError }}</span>
        </label>
        <div v-if="selectedToolSchema" class="rounded border border-slate-800 bg-slate-950 p-3 text-xs text-slate-200">
          <div class="font-semibold">{{ selectedToolSchema.name }}</div>
          <p v-if="selectedToolSchema.description" class="mt-1 text-slate-400">
            {{ selectedToolSchema.description }}
          </p>
          <pre v-if="toolSchemaJSON" class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap leading-tight text-slate-300">{{ toolSchemaJSON }}</pre>
        </div>
      </div>
    </div>
    <div v-else class="rounded-xl border border-dashed border-slate-700 bg-slate-900/40 p-8 text-center text-sm text-slate-400">
      Select a node to edit step details.
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { VueFlow, type Edge, type Node } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'

import {
  fetchWarppTools,
  fetchWarppWorkflow,
  fetchWarppWorkflows,
  saveWarppWorkflow,
} from '@/api/warpp'
import type { WarppStep, WarppTool, WarppWorkflow } from '@/types/warpp'

type LayoutMap = Record<string, { x: number; y: number }>

type StepNodeData = {
  step: WarppStep
  order: number
}

// Ensure data is required on nodes of this kind so we can safely access `node.data.*`
type StepNode = Node<StepNodeData> & { data: StepNodeData }

const nodes = ref<StepNode[]>([])
const edges = ref<Edge[]>([])

const workflowList = ref<WarppWorkflow[]>([])
const selectedIntent = ref<string>('')
const activeWorkflow = ref<WarppWorkflow | null>(null)

const tools = ref<WarppTool[]>([])
const loading = ref(false)
const error = ref('')
const saving = ref(false)

const selectedNodeId = ref<string>('')
const toolArgsError = ref('')

const stepForm = reactive({
  text: '',
  guard: '',
  publishResult: false,
  toolName: '',
  toolArgsText: '',
})

const selectedNode = computed(() => nodes.value.find((node) => node.id === selectedNodeId.value) ?? null)
const selectedToolSchema = computed(() => tools.value.find((tool) => tool.name === stepForm.toolName) ?? null)
const toolSchemaJSON = computed(() => {
  if (!selectedToolSchema.value?.parameters) {
    return ''
  }
  try {
    return JSON.stringify(selectedToolSchema.value.parameters, null, 2)
  } catch (err) {
    console.error('schema stringify failed', err)
    return ''
  }
})

const canSave = computed(() => !!activeWorkflow.value && !saving.value && !toolArgsError.value)

let initializingForm = false

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

watch(selectedIntent, async (intent, _old) => {
  if (!intent) {
    nodes.value = []
    edges.value = []
    activeWorkflow.value = null
    selectedNodeId.value = ''
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

watch(
  () => [stepForm.text, stepForm.guard, stepForm.publishResult, stepForm.toolName, stepForm.toolArgsText],
  () => {
    if (initializingForm) {
      return
    }
    applyFormToNode()
  },
)

watch(selectedNode, (node) => {
  initializingForm = true
  if (!node) {
    stepForm.text = ''
    stepForm.guard = ''
    stepForm.publishResult = false
    stepForm.toolName = ''
    stepForm.toolArgsText = ''
    toolArgsError.value = ''
  } else {
    // use optional chaining to be defensive about the node shape
    stepForm.text = node?.data?.step?.text ?? ''
    stepForm.guard = node?.data?.step?.guard ?? ''
    stepForm.publishResult = !!node?.data?.step?.publish_result
    stepForm.toolName = node?.data?.step?.tool?.name ?? ''
    stepForm.toolArgsText = node?.data?.step?.tool?.args
      ? JSON.stringify(node?.data?.step?.tool?.args, null, 2)
      : ''
    toolArgsError.value = ''
  }
  initializingForm = false
})

function workflowToNodes(wf: WarppWorkflow): StepNode[] {
  const layout = wf.ui?.layout ?? {}
  return wf.steps.map((step, idx) => {
    const stored = layout[step.id]
    return {
      id: step.id,
      type: idx === 0 ? 'input' : idx === wf.steps.length - 1 ? 'output' : undefined,
      position: stored ? { x: stored.x, y: stored.y } : { x: 160, y: idx * 140 },
      data: {
        order: idx as number,
        step: JSON.parse(JSON.stringify(step)) as WarppStep,
      },
      draggable: true,
    }
  })
}

function workflowToEdges(wf: WarppWorkflow): Edge[] {
  const out: Edge[] = []
  for (let i = 1; i < wf.steps.length; i += 1) {
    const prev = wf.steps[i - 1]
    const curr = wf.steps[i]
    out.push({ id: `e-${prev.id}-${curr.id}`, source: prev.id, target: curr.id })
  }
  return out
}

async function loadWorkflow(intent: string) {
  const wf = await fetchWarppWorkflow(intent)
  activeWorkflow.value = wf
  nodes.value = workflowToNodes(wf)
  edges.value = workflowToEdges(wf)
  selectedNodeId.value = wf.steps[0]?.id ?? ''
}

function applyFormToNode() {
  const idx = nodes.value.findIndex((node) => node.id === selectedNodeId.value)
  if (idx === -1) {
    return
  }
  const currentNode = nodes.value[idx]
  const previousStep = currentNode?.data?.step ?? ({} as WarppStep)
  let nextTool: WarppStep['tool']

  if (!stepForm.toolName) {
    nextTool = undefined
    toolArgsError.value = ''
  } else {
    nextTool = { name: stepForm.toolName }
    const trimmed = stepForm.toolArgsText.trim()
    if (!trimmed) {
      toolArgsError.value = ''
    } else {
      try {
        const parsed = JSON.parse(trimmed) as Record<string, any>
        nextTool.args = parsed
        toolArgsError.value = ''
      } catch (err) {
        console.error('tool args parse failed', err)
        toolArgsError.value = 'Tool args must be valid JSON'
      }
    }
  }

  const nextStep: WarppStep = {
    ...previousStep,
    id: currentNode.id,
    text: stepForm.text,
    guard: stepForm.guard.trim() ? stepForm.guard.trim() : undefined,
    publish_result: stepForm.publishResult,
    tool: nextTool,
  }

  const updatedNode: StepNode = {
    ...currentNode,
    data: {
      // ensure data exists and preserve order
      ...(currentNode.data ?? { order: 0 }),
      step: nextStep,
    },
  }

  const updatedNodes = [...nodes.value]
  updatedNodes.splice(idx, 1, updatedNode)
  nodes.value = updatedNodes
  syncWorkflowFromNodes()
}

function syncWorkflowFromNodes() {
  if (!activeWorkflow.value) {
    return
  }
  const orderedNodes = [...nodes.value].sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
  const steps = orderedNodes.map((node) => ({
    ...((node.data?.step) ?? {} as WarppStep),
    id: node.id,
  }))
  activeWorkflow.value = { ...activeWorkflow.value, steps }
}

function onNodeClick(payload: any) {
  if (payload?.node?.id) {
    selectedNodeId.value = payload.node.id
  } else if (payload?.id) {
    selectedNodeId.value = payload.id
  }
}

function onPaneClick() {
  selectedNodeId.value = ''
}

async function onSave() {
  if (!activeWorkflow.value) {
    return
  }
  if (toolArgsError.value) {
    error.value = 'Fix tool argument JSON before saving'
    return
  }
  saving.value = true
  error.value = ''
  try {
    const orderedNodes = [...nodes.value].sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
    const steps = orderedNodes.map((node) => ({
      ...((node.data?.step) ?? {} as WarppStep),
      id: node.id,
    }))
    const layout: LayoutMap = {}
    orderedNodes.forEach((node) => {
      const pos = node.position ?? { x: 0, y: 0 }
      layout[node.id] = { x: pos.x, y: pos.y }
    })
    const payload: WarppWorkflow = {
      ...activeWorkflow.value,
      steps,
      ui: {
        ...(activeWorkflow.value.ui ?? {}),
        layout,
      },
    }
    const prevSelected = selectedNodeId.value
    const saved = await saveWarppWorkflow(payload)
    const listIdx = workflowList.value.findIndex((wf) => wf.intent === saved.intent)
    if (listIdx !== -1) {
      workflowList.value.splice(listIdx, 1, saved)
    } else {
      workflowList.value.push(saved)
    }
    activeWorkflow.value = saved
    nodes.value = workflowToNodes(saved)
    edges.value = workflowToEdges(saved)
    if (prevSelected && saved.steps.some((step) => step.id === prevSelected)) {
      selectedNodeId.value = prevSelected
    } else {
      selectedNodeId.value = saved.steps[0]?.id ?? ''
    }
  } catch (err: any) {
    error.value = err?.message ?? 'Failed to save workflow'
  } finally {
    saving.value = false
  }
}
</script>

<style>
/* ensure flow canvas fills area */
.vue-flow__container { height: 100%; }
</style>
