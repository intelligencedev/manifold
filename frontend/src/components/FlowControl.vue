<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
       class="node-container flow-control-node tool-node"
       @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Mode Selection Dropdown -->
    <div class="input-field">
      <label for="mode-select" class="input-label">Mode:</label>
      <select id="mode-select" v-model="mode" class="input-select">
        <option value="run_count">Run Count</option>
        <option value="loopback">Loopback</option>
      </select>
    </div>

    <!-- Conditional Input Field for Run Count -->
    <div v-if="mode === 'run_count'">
      <div class="input-field">
        <label :for="`${data.id}-runCount`" class="input-label">Run Count:</label>
        <input :id="`${data.id}-runCount`" type="number" v-model.number="runCount" class="input-text" min="1" />
      </div>
    </div>
    
    <!-- Conditional Input Fields for Loopback -->
    <div v-else-if="mode === 'loopback'">
      <div class="input-field">
        <label :for="`${data.id}-loopbackEnabled`" class="input-label">Enable Loopback:</label>
        <input :id="`${data.id}-loopbackEnabled`" type="checkbox" v-model="loopbackEnabled" class="input-checkbox" />
      </div>
      <div class="input-field">
        <label :for="`${data.id}-targetNode`" class="input-label">Target Node ID:</label>
        <input :id="`${data.id}-targetNode`" type="text" v-model="targetNode" class="input-text" />
      </div>
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
                 :line-style="resizeHandleStyle" :width="320" :height="240"
                 :min-width="320" :min-height="240" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'FlowControl_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'FlowControl',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        mode: 'run_count',
        runCount: 1,
        targetNode: '',
        loopbackEnabled: false,  // New loopback parameter
      },
      outputs: {
        result: { output: '' },
      },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '240px',
      },
    }),
  },
})

const { getEdges, findNode } = useVueFlow()

// Computed properties for inputs
const mode = computed({
  get: () => props.data.inputs.mode,
  set: (value) => { props.data.inputs.mode = value },
})

const runCount = computed({
  get: () => props.data.inputs.runCount || 1,
  set: (value) => { props.data.inputs.runCount = value },
})

const targetNode = computed({
  get: () => props.data.inputs.targetNode,
  set: (value) => { props.data.inputs.targetNode = value },
})

const loopbackEnabled = computed({
  get: () => props.data.inputs.loopbackEnabled,
  set: (value) => { props.data.inputs.loopbackEnabled = value },
})

const isHovered = ref(false)
const customStyle = ref({
  width: '320px',
  height: '240px',
})

const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px',
}))

function onResize(event) {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
}

/**
 * The run() function for FlowControl.
 *
 * - In "run_count" mode, the node looks for a connected target node and runs its run() function
 *   the number of times specified by runCount.
 *
 * - In "loopback" mode, the function checks if loopback is enabled and if any connected source node
 *   outputs a TRUE value in outputs.result.output. If so, it cancels the current execution order
 *   and starts from the node ID provided in the targetNode input.
 */
async function run() {
  console.log('Running FlowControl node:', props.id)

  if (mode.value === 'run_count') {
    // Run Count mode: execute the connected target node runCount times.
    const edge = getEdges.value.find(e => e.source === props.id && (!e.sourceHandle || e.sourceHandle === 'continue'))
    if (edge) {
      const target = findNode(edge.target)
      if (target && target.data.run) {
        for (let i = 0; i < runCount.value; i++) {
          await target.data.run()
        }
      } else {
        console.warn('Target node not found or run function missing for edge:', edge)
      }
    } else {
      console.warn('No connected target node found for run_count mode.')
    }
  } else if (mode.value === 'loopback') {
    // Loopback mode: only proceed if loopback is enabled.
    if (!loopbackEnabled.value) {
      console.warn('Loopback is disabled for this node.')
      return;
    }

    // Get connected source nodes
    const connectedSources = getEdges.value
      .filter(edge => edge.target === props.id)
      .map(edge => edge.source)

    let shouldLoopback = false;
    for (const sourceId of connectedSources) {
      const sourceNode = findNode(sourceId);
      if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result) {
        const output = sourceNode.data.outputs.result.output;
        if (output === true || (typeof output === 'string' && output.toLowerCase() === 'true')) {
          shouldLoopback = true;
          break;
        }
      }
    }

    if (shouldLoopback) {
      const target = findNode(targetNode.value);
      if (target && target.data.run) {
        await target.data.run();
      } else {
        console.warn('Target node not found or run function missing for loopback mode.');
      }
    } else {
      console.log('Loopback condition not met (source output is not TRUE).');
    }
  }
}

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})
</script>

<style scoped>
.flow-control-node {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color);
  border: 1px solid var(--node-border-color);
  border-radius: 4px;
  color: var(--node-text-color);
  padding: 10px;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.input-field {
  position: relative;
  margin-bottom: 8px;
}

.input-label {
  font-size: 12px;
}

.input-select,
.input-text {
  margin-top: 5px;
  margin-bottom: 5px;
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-checkbox {
  margin-top: 5px;
}
</style>
