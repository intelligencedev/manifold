<template>
    <div
      :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
      class="node-container flow-control-node tool-node"
      @mouseenter="isHovered = true"
      @mouseleave="isHovered = false"
    >
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- Flow Control Method Selection -->
      <div class="input-field">
        <label :for="`${id}-method`" class="input-label">Flow Control Method:</label>
        <select :id="`${id}-method`" v-model="method" class="input-select">
          <option value="sequential">Sequential</option>
          <option value="parallel">Parallel</option>
          <option value="conditional">Conditional</option>
        </select>
      </div>
  
      <!-- Loop Count Input -->
      <div class="input-field">
        <label :for="`${id}-loopCount`" class="input-label">Loop Count:</label>
        <input
          type="number"
          :id="`${id}-loopCount`"
          v-model.number="loopCount"
          class="input-text"
          min="1"
        />
      </div>
  
      <!-- Aggregated Result Display (Read-only) -->
      <div class="input-field">
        <label :for="`${id}-aggregatedResult`" class="input-label">Aggregated Result:</label>
        <textarea
          :id="`${id}-aggregatedResult`"
          v-model="aggregatedResult"
          class="input-textarea"
          readonly
        ></textarea>
      </div>
  
      <!-- Input Handle (accepts multiple source nodes) -->
      <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" id="input" />
  
      <!-- Output Handles placed on the right -->
      <Handle style="width:10px; height:10px"
        v-if="data.hasOutputs"
        type="source"
        position="right"
        id="continue"
        :style="{ top: '30%' }"
      />
      <Handle style="width:10px; height:10px"
        v-if="data.hasOutputs"
        type="source"
        position="right"
        id="loopback"
        :style="{ top: '70%' }"
      />
  
      <!-- Node Resizer -->
      <NodeResizer
        :is-resizable="true"
        :color="'#666'"
        :handle-style="resizeHandleStyle"
        :line-style="resizeHandleStyle"
        :min-width="200"
        :min-height="100"
        :node-id="id"
        @resize="onResize"
      />
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
      default: 'FlowControl_0'
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'FlowControlNode',
        labelStyle: { fontWeight: 'normal' },
        hasInputs: true,
        hasOutputs: true,
        inputs: {
          // The aggregated output from all source nodes
          aggregatedResult: '',
          // Flow control method: 'sequential' (default), 'parallel', or 'conditional'
          method: 'sequential',
          // Number of loop iterations for loopback
          loopCount: 1
        },
        outputs: {
          // Continue output: for the next nodes in the flow
          continue: '',
          // Loopback output: for nodes that are run in a loop
          loopback: '',
          // Aggregated result output (for downstream nodes expecting aggregated data)
          result: { output: '' }
        },
        style: {
          border: '1px solid #666',
          borderRadius: '4px',
          backgroundColor: '#333',
          color: '#eee',
          width: '200px',
          height: '100px'
        }
      })
    }
  })
  
  const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
  const { getEdges, findNode, updateNodeData } = useVueFlow()
  
  // Local reactive state
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Computed property for showing/hiding the resize handles
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden'
  }))
  
  // Two‐way binding for the flow control method
  const method = computed({
    get: () => props.data.inputs.method,
    set: (value) => {
      props.data.inputs.method = value
      emitUpdate()
    }
  })
  
  // Two‐way binding for the loop count
  const loopCount = computed({
    get: () => props.data.inputs.loopCount,
    set: (value) => {
      props.data.inputs.loopCount = value
      emitUpdate()
    }
  })
  
  // Two‐way binding for the aggregated result
  const aggregatedResult = computed({
    get: () => props.data.inputs.aggregatedResult,
    set: (value) => {
      props.data.inputs.aggregatedResult = value
      emitUpdate()
    }
  })
  
  // Helper to emit updated node data
  const emitUpdate = () => {
    emit('update:data', { id: props.id, data: props.data })
  }
  
  // On mount, set the run() method if not already defined
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  /**
   * The run() method aggregates results from all input (source) nodes,
   * assigns the aggregated result to the continue and loopback outputs,
   * and also updates the result output for downstream nodes.
   * It returns additional control data (the method and loop count).
   */
  async function run() {
    console.log('Running FlowControlNode:', props.id)
  
    // Get all input edges where this node is the target
    const inputEdges = getEdges.value.filter((edge) => edge.target === props.id)
    let aggResult = ''
  
    for (const edge of inputEdges) {
      const sourceNode = findNode(edge.source)
      if (sourceNode && sourceNode.data.outputs) {
        // Try to use a known output structure (e.g. ResponseNode or AgentNode)
        if (sourceNode.data.outputs.result && sourceNode.data.outputs.result.output) {
          aggResult += sourceNode.data.outputs.result.output + ' '
        } else if (sourceNode.data.outputs.response) {
          aggResult += sourceNode.data.outputs.response + ' '
        } else {
          // Fallback: aggregate any string values found in outputs
          for (const key in sourceNode.data.outputs) {
            if (typeof sourceNode.data.outputs[key] === 'string') {
              aggResult += sourceNode.data.outputs[key] + ' '
            }
          }
        }
      }
    }
  
    // Trim and update the aggregated result
    aggResult = aggResult.trim()
    aggregatedResult.value = aggResult
  
    // Update the outputs:
    // The continue output is used for nodes that run after this node.
    // The loopback output is for nodes that run in a loop.
    // Additionally, update the result output so downstream nodes can read the aggregated data.
    props.data.outputs.continue = aggResult
    props.data.outputs.loopback = aggResult
    props.data.outputs.result = { output: aggResult }
  
    console.log('Flow Control Method:', method.value)
    console.log('Loop Count:', loopCount.value)
    console.log('Aggregated Result:', aggResult)
  
    emitUpdate()
    updateNodeData()
  
    return {
      continue: aggResult,
      loopback: aggResult,
      method: method.value,
      loopCount: loopCount.value
    }
  }
  
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }
  </script>
  
  <style scoped>
  .flow-control-node {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    background-color: var(--node-bg-color, #333);
    border: 1px solid var(--node-border-color, #666);
    border-radius: 4px;
    color: var(--node-text-color, #eee);
    padding: 10px;
  }
  
  .node-label {
    font-size: 16px;
    text-align: center;
    margin-bottom: 10px;
    font-weight: bold;
  }
  
  .input-field {
    margin-bottom: 10px;
  }
  
  .input-label {
    display: block;
    font-size: 12px;
    margin-bottom: 4px;
  }
  
  .input-text,
  .input-select,
  .input-textarea {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: 100%;
    box-sizing: border-box;
  }
  </style>
  