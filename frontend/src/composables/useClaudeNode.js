import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useNodeBase } from './useNodeBase'
import { useSystemPromptOptions } from './systemPrompts'

/**
 * Composable for managing ClaudeNode state and functionality
 */
export function useClaudeNode(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow

  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize
  } = useNodeBase(props, emit)

  const { systemPromptOptions } = useSystemPromptOptions()
  
  // Predefined system prompts
  const selectedSystemPrompt = ref("friendly_assistant")
  
  // Computed properties for form binding
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
  

  
  // Node API functionality
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
  
  // Main run function
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
  
  // Event handlers
  
  function handleTextareaMouseEnter() {
    emit('disable-zoom')
  }
  
  function handleTextareaMouseLeave() {
    emit('enable-zoom')
  }
  
  // Lifecycle hooks and watchers
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  // Update system prompt when user picks a new predefined prompt
  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt
    }
  }, { immediate: true })
  
  return {
    // State
    isHovered,
    selectedSystemPrompt,
    
    // Options
    systemPromptOptions,
    
    // Computed properties
    selectedModel,
    models,
    system_prompt,
    user_prompt,
    max_tokens,
    api_key,
    resizeHandleStyle,
    customStyle,
    
    // Methods
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave
  }
}