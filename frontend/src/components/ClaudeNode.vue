<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
         class="node-container claude-node tool-node"
         @mouseenter="isHovered = true" @mouseleave="isHovered = false">
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- Model Selection -->
      <div class="input-field">
        <label :for="`${data.id}-model`" class="input-label">Model:</label>
        <select :id="`${data.id}-model`" v-model="selectedModel" class="input-select">
          <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
        </select>
      </div>
  
      <!-- System Prompt -->
      <div class="input-field">
        <label :for="`${data.id}-system_prompt`" class="input-label">System Prompt (Optional):</label>
        <textarea :id="`${data.id}-system_prompt`" v-model="system_prompt" class="input-textarea"></textarea>
      </div>
  
      <!-- User Prompt -->
      <div class="input-field user-prompt-field">
        <label :for="`${data.id}-user_prompt`" class="input-label">User Prompt:</label>
        <textarea :id="`${data.id}-user_prompt`" v-model="user_prompt" class="input-textarea user-prompt-area"
                  @mouseenter="handleTextareaMouseEnter" @mouseleave="handleTextareaMouseLeave"></textarea>
      </div>
  
      <!-- Max Tokens -->
      <div class="input-field">
        <label :for="`${data.id}-max_tokens`" class="input-label">Max Tokens:</label>
        <input :id="`${data.id}-max_tokens`" type="number" class="input-text" v-model="max_tokens" />
      </div>
  
      <!-- Input/Output Handles -->
      <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
      <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />
  
      <!-- NodeResizer -->
      <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
                   :line-style="resizeHandleStyle" :min-width="380" :min-height="560" :node-id="props.id" @resize="onResize" />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, onMounted, nextTick } from 'vue'
  import { Handle, useVueFlow } from '@vue-flow/core'
  import { NodeResizer } from '@vue-flow/node-resizer'
  const { getEdges, findNode } = useVueFlow()
  
  const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
  const showApiKey = ref(false)
  
  // On mount, attach our run() method to the node’s data if not already set.
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  async function callAnthropicAPI(claudeNode, prompt, systemPrompt) {
    // Find any connected response node.
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target
    const responseNode = responseNodeId ? findNode(responseNodeId) : null
  
    // Clear previous outputs.
    props.data.outputs.response = ''
    if (responseNode) {
      responseNode.data.inputs.response = ''
    }
  
    // Build the request payload.
    const requestBody = {
      model: claudeNode.data.inputs.model,
      max_tokens: parseInt(claudeNode.data.inputs.max_tokens),
      messages: [{ role: 'user', text: prompt }],
      stream: true
    }
    // Ensure the system prompt is sent as an array.
    if (systemPrompt && systemPrompt.trim() !== '') {
      requestBody.system = [systemPrompt.trim()]
    }
  
    // Call the proxy endpoint (note: relative URL so it uses your server).
    const response = await fetch('/api/anthropic/messages', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'x-api-key': claudeNode.data.inputs.api_key,
        'anthropic-version': '2023-06-01'
      },
      body: JSON.stringify(requestBody)
    })
  
    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }
  
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let responseText = ''
  
    // Read chunks from the stream and update the node's output incrementally.
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      const chunk = decoder.decode(value, { stream: true })
      responseText += chunk
      props.data.outputs.response = responseText
      if (responseNode) {
        responseNode.data.inputs.response = responseText
        await nextTick()
        responseNode.run?.()
      }
    }
    return { response: responseText }
  }
  
  async function run() {
    console.log('Running ClaudeNode:', props.id)
    try {
      const claudeNode = findNode(props.id)
      // Clear previous outputs.
      props.data.outputs.response = ''
      let finalPrompt = props.data.inputs.user_prompt
  
      // Gather outputs from any connected source nodes.
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
      if (connectedSources.length > 0) {
        console.log('Connected sources:', connectedSources)
        for (const sourceId of connectedSources) {
          const sourceNode = findNode(sourceId)
          if (sourceNode) {
            console.log('Processing source node:', sourceNode.id)
            finalPrompt += `\n\n${sourceNode.data.outputs.result?.output || ''}`
          }
        }
        console.log('Processed prompt:', finalPrompt)
      }
      return await callAnthropicAPI(claudeNode, finalPrompt, props.data.inputs.system_prompt)
    } catch (error) {
      console.error('Error in ClaudeNode run:', error)
      return { error }
    }
  }
  
  const props = defineProps({
    id: {
      type: String,
      required: true,
      default: 'Claude_0'
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'ClaudeNode',
        labelStyle: { fontWeight: 'normal' },
        hasInputs: true,
        hasOutputs: true,
        inputs: {
          api_key: '',
          model: 'claude-3-5-sonnet-20241022',
          system_prompt: '',
          user_prompt: 'Hello, Claude!',
          max_tokens: 1024
        },
        outputs: { response: '' },
        models: ['claude-3-7-sonnet-latest', 'claude-3-5-sonnet-20241022'],
        style: {
          border: '1px solid #666',
          borderRadius: '12px',
          backgroundColor: '#333',
          color: '#eee',
          width: '380px',
          height: '560px'
        }
      })
    }
  })
  
  const selectedModel = computed({
    get: () => props.data.inputs.model,
    set: (value) => { props.data.inputs.model = value }
  })
  const models = computed({
    get: () => props.data.models,
    set: (value) => { props.data.models = value }
  })
  const system_prompt = computed({
    get: () => props.data.inputs.system_prompt,
    set: (value) => { props.data.inputs.system_prompt = value }
  })
  const user_prompt = computed({
    get: () => props.data.inputs.user_prompt,
    set: (value) => { props.data.inputs.user_prompt = value }
  })
  const max_tokens = computed({
    get: () => props.data.inputs.max_tokens,
    set: (value) => { props.data.inputs.max_tokens = value }
  })
  const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value }
  })
  
  const isHovered = ref(false)
  const customStyle = ref({})
  
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
  }
  
  const handleTextareaMouseEnter = () => emit('disable-zoom')
  const handleTextareaMouseLeave = () => emit('enable-zoom')
  </script>
  
  <style scoped>
  .claude-node {
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
  
  .input-field {
      position: relative;
      margin-bottom: 10px;
  }
  
  .input-label {
      display: block;
      margin-bottom: 4px;
      font-size: 14px;
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
  </style>