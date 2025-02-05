<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container agent-node tool-node"
        @mouseenter="isHovered = true"
        @mouseleave="isHovered = false">
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- Parameters Accordion -->
      <details class="parameters-accordion">
        <summary>Parameters</summary>
  
        <!-- Model Selection -->
        <div class="input-field">
            <label :for="`${data.id}-model`" class="input-label">Model:</label>
            <input type="text" :id="`${data.id}-model`" v-model="model" class="input-text" />
        </div>
  
        <!-- Max Completion Tokens Input -->
        <div class="input-field">
            <label :for="`${data.id}-max_completion_tokens`" class="input-label">Max Completion Tokens:</label>
            <input type="number" :id="`${data.id}-max_completion_tokens`" v-model.number="max_completion_tokens" class="input-text" min="1" />
        </div>
  
        <!-- Temperature Input -->
        <div class="input-field">
            <label :for="`${data.id}-temperature`" class="input-label">Temperature:</label>
            <input type="number" :id="`${data.id}-temperature`" v-model.number="temperature" class="input-text" step="0.1" min="0" max="2" />
        </div>
      </details>
  
      <!-- System Prompt -->
      <div class="input-field">
          <label :for="`${data.id}-system_prompt`" class="input-label">System Prompt:</label>
          <textarea :id="`${data.id}-system_prompt`" v-model="system_prompt" class="input-textarea"></textarea>
      </div>
  
      <!-- User Prompt -->
      <div class="input-field user-prompt-field">
          <label :for="`${data.id}-user_prompt`" class="input-label">User Prompt:</label>
          <textarea :id="`${data.id}-user_prompt`" v-model="user_prompt" class="input-textarea user-prompt-area"
              @mouseenter="handleTextareaMouseEnter" @mouseleave="handleTextareaMouseLeave"></textarea>
      </div>
  
      <!-- Endpoint Input -->
      <div class="input-field">
          <label class="input-label">Endpoint:</label>
          <input type="text" class="input-text" v-model="endpoint" />
      </div>
  
      <!-- OpenAI API Key Input -->
      <div class="input-field">
          <label :for="`${data.id}-api_key`" class="input-label">OpenAI API Key:</label>
          <input :id="`${data.id}-api_key`" :type="showApiKey ? 'text' : 'password'" class="input-text"
              v-model="api_key" />
          <button @click="showApiKey = !showApiKey" class="toggle-password">
              <span v-if="showApiKey">👁️</span>
              <span v-else>🙈</span>
          </button>
      </div>
  
      <!-- Input/Output Handles -->
      <Handle v-if="data.hasInputs" type="target" position="left" />
      <Handle v-if="data.hasOutputs" type="source" position="right" />
  
      <!-- NodeResizer -->
      <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
          :line-style="resizeHandleStyle" :min-width="350" :min-height="560" :node-id="props.id" @resize="onResize" />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, onMounted } from 'vue'
  import { Handle, useVueFlow } from '@vue-flow/core'
  import { NodeResizer } from '@vue-flow/node-resizer'
  
  const { getEdges, findNode, zoomIn, zoomOut, updateNodeData } = useVueFlow()
  
  const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
  
  const showApiKey = ref(false);
  
  // The run() logic
  onMounted(() => {
      if (!props.data.run) {
          props.data.run = run
      }
  })
  
  async function callCompletionsAPI(agentNode, prompt) {
      const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
      const responseNode = responseNodeId ? findNode(responseNodeId) : null;
  
      // Construct the endpoint URL
      let endpoint = agentNode.data.inputs.endpoint + '/completions';
  
      // Helper to detect if the current model is an o1 model.
      const isO1Model = (model) => model.toLowerCase().startsWith("o1" || "o3");
  
      // Build the messages array according to the model type.
      let messages = [];
      if (isO1Model(agentNode.data.inputs.model)) {
          // For o1 models, merge the system prompt into the user prompt.
          messages.push({
              role: "user",
              content: `${agentNode.data.inputs.system_prompt}\n\n${prompt}`,
          });
      } else {
          // For older models, use the standard system + user messages.
          messages.push({
              role: "system",
              content: agentNode.data.inputs.system_prompt,
          });
          messages.push({
              role: "user",
              content: prompt,
          });
      }
  
      const response = await fetch(endpoint, {
          method: "POST",
          headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
          },
          body: JSON.stringify({
              model: agentNode.data.inputs.model,
              max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
              temperature: agentNode.data.inputs.temperature, // TODO: Some OpenAI models dont support this, so a conditional check is needed.
              messages,
              stream: true,
              //reasoning_effort: "high", // TODO: Only for o1 and o3 models, we need to set a condition here to check if the model is o1 or o3.
          }),
      });
  
      if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`API error (${response.status}): ${errorText}`);
      }
  
      const reader = response.body.getReader();
      let buffer = "";
      while (true) {
          const { done, value } = await reader.read();
          if (done) break;
  
          const chunk = new TextDecoder().decode(value);
          buffer += chunk;
  
          let start = 0;
          for (let i = 0; i < buffer.length; i++) {
              if (buffer[i] === "\n") {
                  const line = buffer.substring(start, i).trim();
                  start = i + 1;
  
                  if (line.startsWith("data: ")) {
                      const jsonData = line.substring(6);
                      if (jsonData === "[DONE]") break;
                      try {
                          const parsedData = JSON.parse(jsonData);
                          const delta = parsedData.choices[0]?.delta || {};
  
                          // For older models delta.content will have the token.
                          // For new "thinking" models, delta.thinking may be present.
                          const tokenContent = (delta.content || '') + (delta.thinking || '');
  
                          // Update outputs response
                          props.data.outputs.response += tokenContent;
  
                          if (responseNode) {
                              responseNode.data.inputs.response += tokenContent;
                              responseNode.run;
                          }
                      } catch (e) {
                          console.error("Error parsing response chunk:", e);
                      }
                  }
              }
          }
          buffer = buffer.substring(start);
      }
  
      return { response };
  }
  
  async function run() {
      console.log('Running AgentNode:', props.id);
  
      try {
          const agentNode = findNode(props.id);
          let finalPrompt = props.data.inputs.user_prompt;
  
          // Get connected source nodes
          const connectedSources = getEdges.value
              .filter((edge) => edge.target === props.id)
              .map((edge) => edge.source);
  
          // If there are connected sources, process their outputs
          if (connectedSources.length > 0) {
              console.log('Connected sources:', connectedSources);
              for (const sourceId of connectedSources) {
                  const sourceNode = findNode(sourceId);
                  if (sourceNode) {
                      console.log('Processing source node:', sourceNode.id);
                      finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
                  }
              }
              console.log('Processed prompt:', finalPrompt);
          }
  
          return await callCompletionsAPI(agentNode, finalPrompt);
      } catch (error) {
          console.error('Error in AgentNode run:', error);
          return { error };
      }
  }
  
  const props = defineProps({
      id: {
          type: String,
          required: true,
          default: 'Agent_0',
      },
      data: {
          type: Object,
          required: false,
          default: () => ({
              type: 'AgentNode',
              labelStyle: { fontWeight: 'normal' },
              hasInputs: true,
              hasOutputs: true,
              inputs: {
                  endpoint: '<my_oai_compatible_endpoint>/v1/chat',
                  api_key: '',
                  // For now we provision the backend with a model already loaded. We need to add support for swapping it via this control.
                  // Otherwise, this works with OpenAI models and needs to have a valid model name when using OpenAI.
                  model: 'local',
                  system_prompt: 'You are a helpful assistant.',
                  user_prompt: 'Summarize the following text:',
                  max_completion_tokens: 8192,
                  temperature: 0.6,
              },
              outputs: { response: '' },
              models: ['local', 'chatgpt-4o-latest', 'gpt-4o', 'gpt-4o-mini', 'o1-mini', 'o1', 'o3-mini'],
              style: {
                  border: '1px solid #666',
                  borderRadius: '4px',
                  backgroundColor: '#333',
                  color: '#eee',
                  width: '350px',
                  height: '400px',
              },
          }),
      },
  })
  
  const model = computed({
      get: () => props.data.inputs.model,
      set: (value) => { props.data.inputs.model = value },
  })
  const models = computed({
      get: () => props.data.models,
      set: (value) => { props.data.models = value },
  })
  const system_prompt = computed({
      get: () => props.data.inputs.system_prompt,
      set: (value) => { props.data.inputs.system_prompt = value },
  })
  const user_prompt = computed({
      get: () => props.data.inputs.user_prompt,
      set: (value) => { props.data.inputs.user_prompt = value },
  })
  const endpoint = computed({
      get: () => props.data.inputs.endpoint,
      set: (value) => { props.data.inputs.endpoint = value },
  })
  const api_key = computed({
      get: () => props.data.inputs.api_key,
      set: (value) => { props.data.inputs.api_key = value },
  })
  /** 
   * Computed Property for Max Completion Tokens
   */
  const max_completion_tokens = computed({
      get: () => props.data.inputs.max_completion_tokens,
      set: (value) => { props.data.inputs.max_completion_tokens = value },
  })
  /** End of Computed Property **/
  
  /** 
   * Computed Property for Temperature 
   */
  const temperature = computed({
      get: () => props.data.inputs.temperature,
      set: (value) => { props.data.inputs.temperature = value },
  })
  
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Show/hide the handles
  const resizeHandleStyle = computed(() => ({
      visibility: isHovered.value ? 'visible' : 'hidden',
  }))
  
  // Same approach as in ResponseNode
  function onResize(event) {
      customStyle.value.width = `${event.width}px`
      customStyle.value.height = `${event.height}px`
  }
  
  const handleTextareaMouseEnter = () => {
      emit('disable-zoom')
      zoomIn(0);
      zoomOut(0);
  };
  
  const handleTextareaMouseLeave = () => {
      emit('enable-zoom')
      zoomIn(1);
      zoomOut(1);
  };
  </script>
  
  <style scoped>
  .agent-node {
      /* Make sure we fill the bounding box and use border-box */
      width: 100%;
      height: 100%;
      display: flex;
      flex-direction: column;
      box-sizing: border-box;
  
      background-color: var(--node-bg-color);
      border: 1px solid var(--node-border-color);
      border-radius: 4px;
      color: var(--node-text-color);
  }
  
  .node-label {
      color: var(--node-text-color);
      font-size: 16px;
      text-align: center;
      margin-bottom: 10px;
      font-weight: bold;
  }
  
  /* Matching the pattern from ResponseNode: 
       the rest is your typical input styling, etc. 
  */
  .input-field {
      position: relative;
      /* Add this to make positioning easier for child elements */
  }
  
  .toggle-password {
      position: absolute;
      right: 10px;
      top: 50%;
      transform: translateY(-50%);
      background: none;
      border: none;
      padding: 0;
      cursor: pointer;
  }
  
  .input-text,
  .input-select,
  .input-textarea {
      background-color: #333;
      border: 1px solid #666;
      color: #eee;
      padding: 4px;
      font-size: 12px;
      width: calc(100% - 8px);
      box-sizing: border-box;
  }
  
  .user-prompt-field {
      height: 100%;
      flex: 1;
      /* Let this container take up all remaining height */
      display: flex;
      flex-direction: column;
  }
  
  /* The actual textarea also grows to fill the .user-prompt-field */
  .user-prompt-area {
      flex: 1;
      /* Fill the container's vertical space */
      resize: none;
      /* Prevent user dragging the bottom-right handle inside the textarea */
      overflow-y: auto;
      /* Scroll if the text is bigger than the area */
      min-height: 0;
      /* Prevent flex sizing issues (needed in some browsers) */
  }
  
  /* Basic styling for the parameters accordion */
  .parameters-accordion {
      margin-bottom: 10px;
      border: 1px solid #666;
      border-radius: 4px;
      background-color: #444;
      color: #eee;
      padding: 5px;
  }
  .parameters-accordion summary {
      cursor: pointer;
      padding: 5px;
      font-weight: bold;
  }
  </style>