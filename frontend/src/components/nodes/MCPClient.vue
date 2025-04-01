<template>
    <div :style="data.style" class="node-container tool-node">
      <div class="node-label">
        <div>MCP Client</div>
      </div>
  
      <!-- Textarea for JSON configuration -->
      <div class="input-field">
        <label :for="`${data.id}-config`" class="input-label">Configuration:</label>
        <textarea
          :id="`${data.id}-config`"
          v-model="command"
          @change="updateNodeData"
          class="input-text-area"
          rows="5"
        ></textarea>
      </div>
  
      <!-- Run button to trigger the MCP client execution -->
      <button @click="run">Run MCP Client</button>
  
      <!-- Input and output handles for VueFlow -->
      <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
      <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
    </div>
  </template>
  
  <script setup>
  import { Handle, useVueFlow } from '@vue-flow/core'
  import { computed, onMounted } from 'vue'
  
  const { getEdges, findNode } = useVueFlow()
  
  // Define component props with defaults.
  const props = defineProps({
    id: {
      type: String,
      required: false,
      default: 'MCP_Client_0'
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        style: {},
        type: 'MCPClientNode',
        inputs: {
          // Default configuration payload (in JSON string format)
          command: '{"action": "listTools"}'
        },
        outputs: {},
        hasInputs: true,
        hasOutputs: true,
        inputHandleColor: '#777',
        outputHandleColor: '#777'
      })
    }
  })
  
  // Emit updated data to VueFlow.
  const emit = defineEmits(['update:data'])
  
  // Assign the run() function once the component is mounted.
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  /**
   * The run method performs the following:
   * 1. Clears previous output.
   * 2. Checks if there is connected source data via VueFlow edges.
   * 3. If found, reads that node's output and overwrites the MCP node's input.
   * 4. Parses the (possibly updated) input to create a payload.
   * 5. Ensures the payload includes an action (defaulting to "listTools").
   * 6. Sends the payload to the MCP endpoint and stores the result.
   */
  async function run() {
    try {
      // Clear previous input and output.
      // props.inputs.command = '';
      props.data.outputs.result = '';
  
      // Identify connected source nodes.
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source);
  
      let payload;
  
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0]);
        if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result) {
          const sourceData = sourceNode.data.outputs.result.output;
          console.log('Connected source data:', sourceData);
          try {
            payload = JSON.parse(sourceData);
          } catch (err) {
            payload = { config: sourceData };
          }
          // Overwrite the input field with the connected source's result.
          props.data.inputs.command = JSON.stringify(payload, null, 2);
        }
      } else {
        // If no connected source, parse the user's input.
        let userInput = props.data.inputs.command;
        try {
          payload = JSON.parse(userInput);
        } catch (_err) {
          payload = { config: userInput };
        }
      }
  
      // Ensure payload includes an action; default to "listTools" if missing.
      if (!payload.action) {
        payload.action = 'listTools';
      }
  
      // POST to the MCP execution endpoint (adjust the URL as needed).
      const response = await fetch('http://localhost:8080/api/executeMCP', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
  
      if (!response.ok) {
        const errorMsg = await response.text();
        console.error('Error response from server:', errorMsg);
        props.data.outputs.result = { output: `Error: ${errorMsg}` };
        return { error: errorMsg };
      }
  
      const result = await response.json();
      console.log('MCP Client run result:', result);
  
      // Extract a result string from stdout or stderr, or fallback to full JSON.
      const resultStr = result.stdout || result.stderr || JSON.stringify(result, null, 2);
  
      props.data.outputs = {
        result: { output: resultStr }
      };
  
      updateNodeData();
      return { response, result };
    } catch (error) {
      console.error('Error in run():', error);
      props.data.outputs.result = { output: `Error: ${error.message}` };
      return { error };
    }
  }
  
  // Computed property for the configuration input (two-way binding).
  const command = computed({
    get: () => props.data.inputs?.command || '',
    set: (value) => {
      props.data.inputs.command = value;
      updateNodeData();
    }
  });
  
  // Emit updated node data back to VueFlow.
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { command: command.value },
      outputs: props.data.outputs
    };
    emit('update:data', { id: props.id, data: updatedData });
  }
  </script>
  
  <style scoped>
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
  
  .input-text-area {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    height: auto;
    box-sizing: border-box;
    resize: vertical;
  }
  </style>
