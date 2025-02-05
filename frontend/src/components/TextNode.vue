<template>
    <div
      :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
      class="node-container text-node tool-node"
      @mouseenter="isHovered = true"
      @mouseleave="isHovered = false"
    >
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
  
      <!-- Text input area -->
      <textarea
        v-model="text"
        @change="updateNodeData"
        class="input-textarea"
      ></textarea>
  
      <Handle v-if="data.hasInputs" type="target" position="left" id="input" />
      <Handle v-if="data.hasOutputs" type="source" position="right" id="output" />
  
      <!-- Node resizer for adjusting the node dimensions -->
      <NodeResizer
        :is-resizable="true"
        :color="'#666'"
        :handle-style="resizeHandleStyle"
        :line-style="resizeHandleStyle"
        :min-width="200"
        :min-height="150"
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
      default: 'TextNode_0'
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'TextNode',
        labelStyle: {
          fontWeight: 'bold',
          textAlign: 'center',
          marginBottom: '10px',
          fontSize: '16px'
        },
        style: {
          border: '1px solid var(--node-border-color)',
          borderRadius: '4px',
          backgroundColor: 'var(--node-bg-color)',
          color: 'var(--node-text-color)',
          padding: '10px'
        },
        hasInputs: true,
        hasOutputs: true,
        inputs: {
          text: ''
        },
        outputs: {}
      })
    }
  })
  
  const emit = defineEmits(['update:data', 'resize'])
  const { getEdges, findNode } = useVueFlow()
  
  // When the node is mounted, assign the run() function if not already set
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  // A simple run() function that passes along any text received from a connected source
  async function run() {
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)
  
    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])
      if (sourceNode && sourceNode.data.outputs.result) {
        props.data.inputs.text = sourceNode.data.outputs.result.output
      }
    }
  
    // Set the output equal to the current text input
    props.data.outputs = {
      result: {
        output: text.value
      }
    }
  
    // updateNodeData()
    // return { output: text.value }
  }
  
  // Computed property for two-way binding of the text input
  const text = computed({
    get: () => props.data.inputs.text,
    set: (value) => {
      props.data.inputs.text = value
      updateNodeData()
    }
  })
  
  // Emit updated node data back to VueFlow
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { text: text.value },
      outputs: props.data.outputs
    }
    emit('update:data', { id: props.id, data: updatedData })
  }
  
  // Custom style for handling resizes
  const customStyle = ref({})
  
  // Track whether the node is hovered (to show/hide the resize handles)
  const isHovered = ref(false)
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden'
  }))
  
  // Handle the resize event to update the node dimensions
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    // Also update the node's style data so it persists
    props.data.style.width = `${event.width}px`
    props.data.style.height = `${event.height}px`
    updateNodeData()
    emit('resize', { id: props.id, width: event.width, height: event.height })
  }
  </script>
  
<style scoped>
.node-container {
    display: flex;
    flex-direction: column;
    position: relative;
    box-sizing: border-box;
    justify-content: center;
    align-items: center;
    padding: 10px;
}

.text-node {
    background-color: var(--node-bg-color);
    border: 1px solid var(--node-border-color);
    border-radius: 4px;
    color: var(--node-text-color);
    width: 100%;
    height: 100%;
}

.node-label {
    color: var(--node-text-color);
    font-size: 16px;
    text-align: center;
    margin-bottom: 10px;
    font-weight: bold;
}

/* Consistent styling for input elements */
.input-textarea {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 10px;
    font-size: 14px;
    width: calc(100% - 20px);
    box-sizing: border-box;
    resize: vertical;
    flex-grow: 1;
    margin: 10px 0;
}
</style>
  