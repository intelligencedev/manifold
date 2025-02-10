<template>
    <div :style="data.style" class="node-container tool-node">
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- Filename Input -->
      <div class="input-field">
        <label :for="`${data.id}-filename`" class="input-label">Filename:</label>
        <input
          :id="`${data.id}-filename`"
          type="text"
          v-model="filename"
          @change="updateNodeData"
          class="input-text"
        />
      </div>
  
      <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" id="input" />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, watch, onMounted } from 'vue';
  import { Handle, useVueFlow } from '@vue-flow/core';
  
  const props = defineProps({
    id: {
      type: String,
      required: true,
      default: 'SaveText_0',
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'saveTextNode',
        labelStyle: {},
        style: {},
        inputs: {
          filename: 'output.md',
          text: '',
        },
        hasInputs: true,
        hasOutputs: false,
        inputHandleColor: '#777',
        outputHandleShape: '50%',
        handleColor: '#777',
      }),
    },
  });
  
  const emit = defineEmits(['update:data']);
  const { getEdges, findNode } = useVueFlow();
  
  const filename = computed({
    get: () => props.data.inputs.filename,
    set: (value) => {
      props.data.inputs.filename = value;
    },
  });
  
  watch(filename, () => {
    updateNodeData();
  });
  
  watch(
    () => props.data,
    (newData) => {
      filename.value = newData.inputs?.filename || 'output.md';
      emit('update:data', { id: props.id, data: newData });
    },
    { deep: true }
  );
  
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run;
    }
  });
  
  async function run() {
    console.log('Running SaveTextNode:', props.id);
  
    // Find the connected source node
    const connectedEdges = getEdges.value.filter((edge) => edge.target === props.id);
    if (connectedEdges.length === 0) {
      console.warn('SaveTextNode has no input connection.');
      return;
    }
  
    const sourceNode = findNode(connectedEdges[0].source);
    if (!sourceNode || !sourceNode.data?.outputs?.result?.output) {
      console.warn('Source node output not found.');
      return;
    }
  
    const content = sourceNode.data.outputs.result.output;
    // const finalFilename = props.data.inputs.filename.endsWith('.md')
    //   ? props.data.inputs.filename
    //   : `${props.data.inputs.filename}.md`;

    const finalFilename = props.data.inputs.filename;
  
    try {
      const response = await fetch('http://localhost:8080/api/save-file', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          filepath: finalFilename,
          content: content
        })
      });
  
      if (!response.ok) {
        const errorData = await response.json();
        console.error('Error saving markdown content:', errorData.error);
      } else {
        console.log('Content saved to:', finalFilename);
      }
    } catch (error) {
      console.error('Error saving file:', error);
    }
  }
  
  const updateNodeData = () => {
    emit('update:data', { id: props.id, data: { ...props.data } });
  };
  </script>
  
  <style scoped>
  .node-container {
    border: 3px solid var(--node-border-color) !important;
    background-color: var(--node-bg-color) !important;
    box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
    padding: 15px;
    border-radius: 8px;
    color: var(--node-text-color);
    font-family: 'Roboto', sans-serif;
  }
  
  .tool-node {
    --node-border-color: #777 !important;
    --node-bg-color: #1e1e1e !important;
    --node-text-color: #eee;
  }
  
  .node-label {
    color: var(--node-text-color);
    font-size: 16px;
    text-align: center;
    margin-bottom: 10px;
    font-weight: bold;
  }
  
  .input-field {
    margin-bottom: 8px;
  }
  
  .input-text {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
  }
  
  .handle-input,
  .handle-output {
    width: 12px;
    height: 12px;
    border: none;
    background-color: #777;
  }
  </style>
  