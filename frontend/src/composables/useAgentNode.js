import { ref, computed, watch, onMounted } from 'vue'
import { useConfigStore } from '@/stores/configStore'
import { useCompletionsApi } from './useCompletionsApi'
import { useCodeEditor } from './useCodeEditor'

/**
 * Composable for managing AgentNode state and functionality
 */
export function useAgentNode(props, emit) {
  const configStore = useConfigStore()
  const { callCompletionsAPI } = useCompletionsApi()
  const { setEditorCode } = useCodeEditor()
  
  // State variables
  const showApiKey = ref(false)
  const enableToolCalls = ref(false)
  const isHovered = ref(false)
  const customStyle = ref({
    width: '380px',
    height: '760px'
  })
  
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
    },
    retrieval_assistant: {
      role: "Retrieval Assistant",
      system_prompt: `You are capable of executing available function(s) and should always use them.
Always ask for the required input to: recipient==all.
Use JSON for function arguments.
Respond using the following format:
>>>\${recipient}
\${content}

Available functions:
namespace functions {
  // retrieves documents related to the topic
  type combined_retrieve = (_: {
    query: string,
    file_path_filter?: string,
    limit?: number,
    use_inverted_index?: boolean,
    use_vector_search?: boolean,
    merge_mode?: string,
    return_full_docs?: boolean,
    rerank?: boolean,
    alpha?: number,
    beta?: number
  }) => any;
  
  // remembers previous chats
  type agentic_retrieve = (_: {
    query: string,
    limit?: number
  }) => any;
}
`
    },
    recursive_agent: {
      role: "Recursive Agent",
      system_prompt: `Below is a list of file system, Git, and agent operations you can perform. Choose the best output to answer the user's query:

      - Required Fields:
      - "tool": The name of the tool you wish to execute. This can be one of:
          "agent".
      - "args": A JSON object containing the arguments required by the tool.
      - Payload Examples:
          { "action": "execute", "tool": "agent", "args": { "query": "Your query here", "maxCalls": 100 } }

      Important: You ONLY use the agent tool!

      You NEVER respond using Markdown. You ALWAYS respond using raw JSON choosing the best tool to answer the user's query.
      ALWAYS use the following raw JSON structure (for example for the time tool): { "action": "execute", "tool": "time", "args": {} }
      REMEMBER TO NEVER use markdown formatting and ONLY use raw JSON.`
    },
    tool_calling: {
      role: "Tool Caller",
      system_prompt: `Below is a list of tools and agent operations you can perform. Choose the best tool to answer the user's query. For example:

      - Required Fields:
      - "tool": The name of the tool you wish to execute. This can be one of:
          "agent".
      - "args": A JSON object containing the arguments required by the tool.
      - Payload Examples:
          { "action": "execute", "tool": "agent", "args": { "query": "Your query here", "maxCalls": 15 } }

      You NEVER respond using Markdown. You ALWAYS respond using raw JSON choosing the best tool to answer the user's query.
      ALWAYS use the following raw JSON structure (for example for the time tool): { "action": "execute", "tool": "time", "args": {} }
      REMEMBER TO NEVER use markdown formatting and ONLY use raw JSON.`
    },
  }
  
  // Computed properties for form binding
  const model = computed({
    get: () => props.data.inputs.model,
    set: (value) => { props.data.inputs.model = value },
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
  
  const max_completion_tokens = computed({
    get: () => props.data.inputs.max_completion_tokens,
    set: (value) => { props.data.inputs.max_completion_tokens = value },
  })
  
  const temperature = computed({
    get: () => props.data.inputs.temperature,
    set: (value) => { props.data.inputs.temperature = value },
  })
  
  // Provider options and selection
  const providerOptions = [
    { value: 'llama-server', label: 'llama-server' },
    { value: 'mlx_lm.server', label: 'mlx_lm.server' },
    { value: 'openai', label: 'openai' }
  ]
  
  // Transform system prompt options into usable format for BaseSelect
  const systemPromptOptionsList = computed(() => {
    return Object.entries(systemPromptOptions).map(([key, value]) => ({
      value: key,
      label: value.role
    }));
  });
  
  // Provider detection and setting
  const provider = computed({
    get: () => {
      if (props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions') {
        return 'openai';
      } else if (props.data.inputs.endpoint === configStore.config?.Completions?.DefaultHost) {
        if (configStore.config?.Completions?.Provider === 'llama-server') {
          return 'llama-server';
        } else if (configStore.config?.Completions?.Provider === 'mlx_lm.server') {
          return 'mlx_lm.server';
        }
      }
      return 'llama-server';
    },
    set: (value) => {
      if (value === 'openai') {
        props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
      } else {
        props.data.inputs.endpoint = configStore.config?.Completions?.DefaultHost || '';
      }
    }
  });
  
  // Styling and UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  const computedContainerStyle = computed(() => ({
    ...props.data.style,
    ...customStyle.value,
    width: '100%',
    height: '100%'
  }))
  
  // Node functionality
  async function run() {
    console.log('Running AgentNode:', props.id);
    try {
      let finalPrompt = props.data.inputs.user_prompt;
      
      // Collect input from connected nodes if any
      if (props.vueFlowInstance) {
        const { getEdges, findNode } = props.vueFlowInstance;
        const connectedSources = getEdges.value
          .filter((edge) => edge.target === props.id)
          .map((edge) => edge.source);
  
        if (connectedSources.length > 0) {
          for (const sourceId of connectedSources) {
            const sourceNode = findNode(sourceId);
            if (sourceNode) {
              finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
            }
          }
        }
      }
      
      // Configuration for the API call
      const agentConfig = {
        provider: provider.value,
        endpoint: props.data.inputs.endpoint,
        api_key: props.data.inputs.api_key,
        model: props.data.inputs.model,
        system_prompt: props.data.inputs.system_prompt,
        max_completion_tokens: props.data.inputs.max_completion_tokens,
        temperature: props.data.inputs.temperature,
        enableToolCalls: enableToolCalls.value
      };
      
      // Reset the response
      props.data.outputs.response = '';
      props.data.outputs.error = null;
      
      // Handle response updates
      const onResponseUpdate = (tokenContent, fullResponse) => {
        props.data.outputs.response = fullResponse;
        
        // Update connected output nodes if any
        if (props.vueFlowInstance) {
          const { getEdges, findNode } = props.vueFlowInstance;
          const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
          const responseNode = responseNodeId ? findNode(responseNodeId) : null;
          
          if (responseNode) {
            responseNode.data.inputs.response = fullResponse;
            responseNode.run();
          }
        }
      };
      
      // Call the API
      const result = await callCompletionsAPI(agentConfig, finalPrompt, onResponseUpdate);

      // Handle error in result
      if (result.error) {
        props.data.outputs.error = result.error;
        props.data.outputs.response = JSON.stringify({ error: result.error }, null, 2);
        
        // Also update connected response nodes with the error
        if (props.vueFlowInstance) {
          const { getEdges, findNode } = props.vueFlowInstance;
          const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
          const responseNode = responseNodeId ? findNode(responseNodeId) : null;
          
          if (responseNode) {
            responseNode.data.inputs.response = props.data.outputs.response;
            responseNode.run();
          }
        }
      }
      
      return result;
    } catch (error) {
      console.error('Error in AgentNode run:', error);
      const errorMessage = error.message || "Unknown error occurred";
      
      // Store error in outputs
      props.data.outputs.error = errorMessage;
      props.data.outputs.response = JSON.stringify({ error: errorMessage }, null, 2);
      
      // Update connected response nodes with the error
      if (props.vueFlowInstance) {
        const { getEdges, findNode } = props.vueFlowInstance;
        const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
        const responseNode = responseNodeId ? findNode(responseNodeId) : null;
        
        if (responseNode) {
          responseNode.data.inputs.response = props.data.outputs.response;
          responseNode.run();
        }
      }
      
      return { error: errorMessage };
    }
  }
  
  /**
   * Sends code to the code editor
   * Extracts code from node output and sends it to the editor
   */
  function sendToCodeEditor() {
    if (props.data.outputs && props.data.outputs.response) {
      // Check for code blocks with ```js or ```javascript delimiters
      const codeBlockRegex = /```(?:js|javascript)\s*([\s\S]*?)```/g;
      const matches = [...props.data.outputs.response.matchAll(codeBlockRegex)];
      
      if (matches.length > 0) {
        // Use the first JavaScript code block found
        setEditorCode(matches[0][1].trim());
      } else {
        // If no specific code blocks are found, check for any code blocks
        const anyCodeBlockRegex = /```([\s\S]*?)```/g;
        const anyMatches = [...props.data.outputs.response.matchAll(anyCodeBlockRegex)];
        
        if (anyMatches.length > 0) {
          setEditorCode(anyMatches[0][1].trim());
        } else {
          // If no code blocks at all, send the entire output
          setEditorCode(props.data.outputs.response);
        }
      }
    }
  }

  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    emit('resize', event);
  }
  
  function handleTextareaMouseEnter() {
    emit('disable-zoom');
    if (props.vueFlowInstance) {
      const { zoomIn, zoomOut } = props.vueFlowInstance;
      zoomIn(0);
      zoomOut(0);
    }
  }
  
  function handleTextareaMouseLeave() {
    emit('enable-zoom');
    if (props.vueFlowInstance) {
      const { zoomIn, zoomOut } = props.vueFlowInstance;
      zoomIn(1);
      zoomOut(1);
    }
  }
  
  // Lifecycle hooks and watchers
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run;
    }
  })
  
  // Use config for default key & endpoint
  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig && newConfig.Completions) {
        if (!props.data.inputs.api_key) {
          props.data.inputs.api_key = newConfig.Completions.APIKey;
        }
        if (!props.data.inputs.endpoint) {
          props.data.inputs.endpoint = newConfig.Completions.DefaultHost;
        }
      }
    },
    { immediate: true }
  )
  
  // If the store's provider changes, reset endpoint
  watch(() => configStore.config?.Completions?.Provider, (newProvider) => {
    if (newProvider && provider.value !== 'openai') {
      props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
  }, { immediate: true });
  
  // Update system prompt when user picks a new predefined prompt
  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  }, { immediate: true });
  
  return {
    // State
    showApiKey,
    enableToolCalls,
    selectedSystemPrompt,
    isHovered,
    
    // Options
    systemPromptOptions,
    systemPromptOptionsList,
    providerOptions,
    
    // Computed properties
    provider,
    endpoint,
    api_key,
    model,
    max_completion_tokens,
    temperature,
    system_prompt,
    user_prompt,
    resizeHandleStyle,
    computedContainerStyle,
    
    // Methods
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    sendToCodeEditor
  }
}