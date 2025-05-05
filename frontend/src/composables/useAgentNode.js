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
  const agentMode = ref(false)
  const isHovered = ref(false)
  const customStyle = ref({
    width: '380px',
    height: '760px'
  })
  
  // Helper function for calling the agent API
  async function callAgentAPI({ endpoint, objective, model, maxSteps = 30 }) {
    const res = await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ objective, max_steps: maxSteps, model }),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(`Agent API ${res.status}: ${errText}`);
    }
    return res.json(); // { session_id, trace, result, completed }
  }

  // Helper function to create an event stream splitter
  function createEventStreamSplitter() {
    let buffer = '';
    return new TransformStream({
      transform(chunk, controller) {
        buffer += chunk;
        let idx;
        while ((idx = buffer.indexOf("\n\n")) !== -1) {
          const event = buffer.slice(0, idx).replace(/^data:\s*/gm,'').trim();
          controller.enqueue(event);
          buffer = buffer.slice(idx + 2);
        }
      }
    });
  }
  
  // Predefined system prompts
  const selectedSystemPrompt = computed({
    get: () => props.data.selectedSystemPrompt || "",
    set: (val) => { props.data.selectedSystemPrompt = val },
  });
  
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
    code_node: {
      role: "Code Execution Node",
      system_prompt: `You are an expert in generating JSON payloads for executing code with dynamic dependency installation in a sandbox environment. The user can request code in one of three languages: python, go, or javascript. If the user requests a language outside of these three, respond with the text:

'language not supported'
Otherwise, produce a valid JSON object with the following structure:

language: a string with the value "python", "go", or "javascript".

code: a string containing the code that should be run in the specified language.

dependencies: an array of strings, where each string is the name of a required package or library.

If no dependencies are needed, the dependencies array must be empty (e.g., []).

Always return only the raw JSON string without any additional text, explanation, or markdown formatting. If the requested language is unsupported, return only language not supported without additional formatting.`
    },
    webgl_node: {
      role: "WebGL Node",
      system_prompt: "You are to generate a JSON payload for a WebGLNode component that renders a triangle. The JSON must contain exactly two keys:\n\n\"vertexShader\"\n\"fragmentShader\"\nRequirements for the Shaders:\n\nVertex Shader:\nMust define a vertex attribute named a_Position (i.e. attribute vec2 a_Position;).\nMust transform this attribute into clip-space coordinates, typically using a line such as gl_Position = vec4(a_Position, 0.0, 1.0);.\nFragment Shader:\nShould use valid WebGL GLSL code.\nOptionally, if you need to compute effects based on the canvas dimensions, you may include a uniform named u_resolution. This uniform will be automatically set to the canvas dimensions by the WebGLNode.\nEnsure that the code produces a visible output (for example, rendering a colored triangle).\nAdditional Guidelines:\n\nThe generated JSON must be valid (i.e. parseable as JSON).\nDo not include any extra keys beyond \"vertexShader\" and \"fragmentShader\".\nEnsure that all GLSL code is valid for WebGL.\nExample Outline:\n\n{\n  \"vertexShader\": \"attribute vec2 a_Position; void main() { gl_Position = vec4(a_Position, 0.0, 1.0); }\",\n  \"fragmentShader\": \"precision mediump float; uniform vec2 u_resolution; void main() { /* shader code */ }\"\n}\n\nDO NOT format as markdown. DO NOT wrap code in code blocks or back ticks. You MUST always ONLY return the raw JSON.",
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
    planning_agent: {
      role: "Planning Agent",
      system_prompt: `You are **Planner-Agent**.  
Your job is to break the user’s request into an ordered list of executable steps for the MCP client.

────────────────────────────────────────────────────────
STEP TYPES
────────────────────────────────────────────────────────

1. **Tool call** – one line containing **only** a raw JSON object with these keys in this order:

{
  "server":   "<serverName>",
  "tool":     "<toolName>",          // must exist in the provided tool list
  "endpoint": "<fullURL or empty>",  // not required for all tools, ensure to follow proper schema
  "args": { … }                      // only the parameters that tool accepts
}

• If any required argument value is unknown, use the placeholder string \`"FILL_ME_IN"\`.  
• Do **not** invent keys that are not defined in the tool’s schema.

2. **Reasoning / summarisation step** – a short imperative sentence (e.g. \`Summarise TODOs and determine length flags\`).  
  *These steps are purely for the executor’s information; they do not call a tool.*

────────────────────────────────────────────────────────
OUTPUT FORMAT
────────────────────────────────────────────────────────

* Produce **one line total**, where steps are separated by the delimiter \`|||\`:  
  \`step 1, step 2, step 3, …\`
* No markdown, no code fences, no commentary before or after.
* Each step may include internal spaces; the \`|||\` alone delimit the steps.
* Generate only the minimal sequence needed to satisfy the user’s query.
* Assume an EXECUTOR agent will process one step at a time; you do **not** execute tools yourself.

────────────────────────────────────────────────────────
EXAMPLE
────────────────────────────────────────────────────────

{"path":"/tmp","maxDepth":10,"server":"manifold","tool":"directory_tree"}|||
Summarise TODO comments and flag any over 80 characters

────────────────────────────────────────────────────────
Follow these rules **exactly** for every plan you produce.
`
    },
    tool_calling: {
      role: "Tool Caller",
      system_prompt: `You are a specialized LLM assistant designed to generate JSON payloads for various servers (SecurityTrails, GitHub, Manifold).

Your ONLY job is to take user queries about data or actions related to these servers and convert them into the correct JSON payload format. You must ONLY return the raw JSON payload without any explanations or markdown formatting.

The payload must ALWAYS follow this structure:

{
  "server": "serverName",            // "securitytrails", "github", or "manifold"
  "tool":   "toolName",              // Name of the tool to invoke
  "endpoint": "https://api.securitytrails.com/v2/projects/PROJECT_ID/assets/_search",  // Full URL including base URL and path parameters
  "args": {                           // Only include required and provided parameters
    // For SecurityTrails, always include the original parameters such as:
    // "project_id": "12345",
    // "asset_id": "www.example.com"
    // These are used by the caller to construct the endpoint URL
  }
}

Rules:
- Only output the JSON payload object and nothing else.
- Do not wrap in code blocks, backticks, or any extra formatting.
- The "endpoint" key MUST contain the full URL with the base URL (e.g., https://api.securitytrails.com) AND path INCLUDING any URL parameters substituted into the path.
- When using SecurityTrails API, you must still include path parameters like "project_id" and "asset_id" in the "args" object even though they are also used in the endpoint URL.
- Include only the fields under "args" that are necessary for the given tool; use placeholders (e.g., "FILL_ME_IN") for any missing required parameters.
- Correctly format nested objects or arrays in "args" when needed.
- Do not add commentary, explanations, or additional keys.

Example for SecurityTrails:
{
  "server": "securitytrails",
  "tool": "read_asset",
  "endpoint": "https://api.securitytrails.com/v2/projects/abc123/assets/example.com",
  "args": {
    "project_id": "abc123",
    "asset_id": "example.com",
    "additional_fields": ["dns", "whois"]
  }
}
`},
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
      
      // Custom local endpoint case - check for localhost or common patterns
      if (props.data.inputs.endpoint?.includes('localhost') ||
          props.data.inputs.endpoint?.includes('127.0.0.1')) {
        // Maintain the last selected local provider or default to llama-server
        if (props.data._lastLocalProvider === 'mlx_lm.server') {
          return 'mlx_lm.server';
        }
        return 'llama-server';
      }
      
      return 'llama-server';
    },
    set: (value) => {
      // Store the last selected local provider for reference
      if (value !== 'openai') {
        props.data._lastLocalProvider = value;
      }
      
      // Only change the endpoint when switching to OpenAI
      if (value === 'openai') {
        props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
      } else if (!props.data.inputs.endpoint || props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions') {
        // Only set the default local endpoint if current endpoint is empty or OpenAI endpoint
        props.data.inputs.endpoint = configStore.config?.Completions?.DefaultHost || 'http://localhost:32186/v1/chat/completions';
      }
      // Otherwise, keep the user's custom endpoint
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
        temperature: props.data.inputs.temperature,
        system_prompt: props.data.inputs.system_prompt,
        max_completion_tokens: props.data.inputs.max_completion_tokens,
        enableToolCalls: enableToolCalls.value
      };
      
      // Only include model when using OpenAI endpoint
      if (props.data.inputs.endpoint && props.data.inputs.endpoint.includes('api.openai.com')) {
        agentConfig.model = props.data.inputs.model;
      }
      
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
            //responseNode.run();
          }
        }
      };
      
      // Call the API based on the mode
      let result;
      
      if (agentMode.value) {
        // --- ReAct agent call with streaming ---
        const sseResp = await fetch('/api/agents/react/stream', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json', 
            'Accept': 'text/event-stream'
          },
          body: JSON.stringify({ 
            objective: finalPrompt, 
            max_steps: 30, 
            model: provider.value === 'openai' ? props.data.inputs.model : '' 
          })
        });
        
        if (!sseResp.ok) {
          throw new Error(`SSE ${sseResp.status}`);
        }

        const reader = sseResp.body
              .pipeThrough(new TextDecoderStream())
              .pipeThrough(createEventStreamSplitter())
              .getReader();

        let accumulatedThoughts = '';  // accumulate just the thought content
        let finalResult = '';          // store the final result separately

        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          if (value === '[[EOF]]') continue;
          
          // Extract content from <think> tags
          const thinkMatch = value.match(/<think>([\s\S]*?)<\/think>/);
          if (thinkMatch) {
            // This is a thought chunk, add to accumulated thoughts
            accumulatedThoughts += thinkMatch[1] + '\n';
          } else {
            // This is non-thought content, it's the final result/summary
            finalResult = value;
          }
          
          // Combine into a response with a think block + any final result
          const combinedResponse = 
            (accumulatedThoughts ? `<think>${accumulatedThoughts}</think>` : '') + 
            (finalResult ? `\n${finalResult}` : '');
            
          onResponseUpdate(combinedResponse, combinedResponse);
        }
        
        // Set the final result with proper formatting
        const finalResponse = 
          (accumulatedThoughts ? `<think>${accumulatedThoughts}</think>` : '') + 
          (finalResult ? `\n${finalResult}` : '');
          
        result = { content: finalResponse };
      } else {
        result = await callCompletionsAPI(agentConfig, finalPrompt, onResponseUpdate);
      }

      // Store result in outputs structure for downstream nodes
      props.data.outputs = {
        ...props.data.outputs,
        result: {
          output: result.content || result.error || props.data.outputs.response
        }
      };

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
      // Update only if dropdown was manually changed, not on restore
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  });
  
  // Optional: preset the agent endpoint when toggling agent mode
  watch(agentMode, (on) => {
    if (on && !props.data.inputs.endpoint?.includes('/api/agents/react')) {
      // Set the regular endpoint - the stream endpoint will be used internally
      props.data.inputs.endpoint = 'http://localhost:8080/api/agents/react';
    }
  });
  
  return {
    // State
    showApiKey,
    enableToolCalls,
    agentMode,
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