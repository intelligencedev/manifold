import { ref, computed, watch, onMounted, nextTick } from 'vue'

/**
 * Composable for managing ClaudeNode state and functionality
 */
export function useClaudeNode(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Predefined system prompts
  const selectedSystemPrompt = ref("friendly_assistant")
  const systemPromptOptions = {
    friendly_assistant: {
      role: "Friendly Assistant",
      system_prompt:
        "You are a helpful, friendly, and knowledgeable general-purpose AI assistant. You can answer questions, provide information, engage in conversation, and assist with a wide variety of tasks.  Be concise in your responses when possible, but prioritize clarity and accuracy.  If you don't know something, admit it.  Maintain a conversational and approachable tone."
    },
    search_assistant: {
      role: "Search Assistant",
      system_prompt:
        "You are a helpful assistant that specializes in generating effective search engine queries.  Given any text input, your task is to create one or more concise and relevant search queries that would be likely to retrieve information related to that text from a search engine (like Google, Bing, etc.).  Consider the key concepts, entities, and the user's likely intent.  Prioritize clarity and precision in the queries."
    },
    research_analyst: {
      role: "Research Analyst",
      system_prompt:
        "You are a skilled research analyst with deep expertise in synthesizing information. Approach queries by breaking down complex topics, organizing key points hierarchically, evaluating evidence quality, providing multiple perspectives, and using concrete examples. Present information in a structured format with clear sections, use bullet points for clarity, and visually separate different points with markdown. Always cite limitations of your knowledge and explicitly flag speculation."
    },
    creative_writer: {
      role: "Creative Writer",
      system_prompt:
        "You are an exceptional creative writer. When responding, use vivid sensory details, emotional resonance, and varied sentence structures. Organize your narratives with clear beginnings, middles, and ends. Employ literary techniques like metaphor and foreshadowing appropriately. When providing examples or stories, ensure they have depth and authenticity. Present creative options when asked, rather than single solutions."
    },
    code_expert: {
      role: "Programming Expert",
      system_prompt:
        "You are a senior software developer with expertise across multiple programming languages. Present code solutions with clear comments explaining your approach. Structure responses with: 1) Problem understanding 2) Solution approach 3) Complete, executable code 4) Explanation of how the code works 5) Alternative approaches. Include error handling in examples, use consistent formatting, and provide explicit context for any code snippets. Test your solutions mentally before presenting them."
    },
    teacher: {
      role: "Educational Expert",
      system_prompt:
        "You are an experienced teacher skilled at explaining complex concepts. Present information in a structured, progressive manner from foundational to advanced. Use analogies and examples to connect new concepts to familiar ones. Break down complex ideas into smaller components. Incorporate multiple formats (definitions, examples, diagrams described in text) to accommodate different learning styles. Ask thought-provoking questions to deepen understanding. Anticipate common misconceptions and address them proactively."
    },
    data_analyst: {
      role: "Data Analysis Expert",
      system_prompt:
        "You are a data analysis expert. When working with data, focus on identifying patterns and outliers, considering statistical significance, and exploring causal relationships vs. correlations. Present your analysis with a clear narrative structure that connects data points to insights. Use hypothetical data visualization descriptions when relevant. Consider alternative interpretations of data and potential confounding variables. Clearly communicate limitations and assumptions in any analysis."
    }
  }
  
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
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
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
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }
  
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