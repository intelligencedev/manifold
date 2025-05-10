import { ref, computed, watch, onMounted } from 'vue'
import { useConfigStore } from '@/stores/configStore'
import { useCodeEditor } from './useCodeEditor'

/**
 * Composable for managing AgentNode state and functionality
 */
export function useAgentNode(props, emit) {
  const configStore = useConfigStore()
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
  
  // Computed property for agent max steps
  const agentMaxSteps = computed(() => 
    configStore.config?.Completions?.Agent?.MaxSteps || 30
  )

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
      },
      flush(controller) {
        if (buffer.trim()) {
          const event = buffer.replace(/^data:\s*/gm,'').trim();
          if (event) {
            controller.enqueue(event);
          }
        }
      }
    });
  }
  
  // Function to update response in real-time as agent thoughts come in
  function onResponseUpdate(content, fullResponse) {
    // Update the UI with the streamed response
    props.data.outputs = {
      ...props.data.outputs,
      response: content,
      result: {
        output: fullResponse
      }
    };
    
    // Also update any connected response nodes
    if (props.vueFlowInstance) {
      const { getEdges, findNode } = props.vueFlowInstance;
      const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
      const responseNode = responseNodeId ? findNode(responseNodeId) : null;
      
      if (responseNode) {
        responseNode.data.inputs.response = content;
      }
    }
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
    data_analyst: {
      role: "Data Analysis Expert",
      system_prompt:
        "You are a data analysis expert. When working with data, focus on identifying patterns and outliers, considering statistical significance, and exploring causal relationships vs. correlations. Present your analysis with a clear narrative structure that connects data points to insights. Use hypothetical data visualization descriptions when relevant. Consider alternative interpretations of data and potential confounding variables. Clearly communicate limitations and assumptions in any analysis."
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
            max_steps: agentMaxSteps.value,
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

        try {
          while (true) {
            const { value, done } = await reader.read();
            if (done) break;
            if (value === '[[EOF]]') {
              // tell the stream we're done
              await reader.cancel();
              break;
            }
            // Extract content from <think> tags
            const thinkMatch = value.match(/<think>([\s\S]*?)<\/think>/);
            if (thinkMatch) {
              accumulatedThoughts += thinkMatch[1] + '\n';
            } else {
              finalResult = value;
            }
            const combinedResponse = 
              (accumulatedThoughts ? `<think>${accumulatedThoughts}</think>` : '') + 
              (finalResult ? `\n${finalResult}` : '');
            onResponseUpdate(combinedResponse, combinedResponse);
          }
        } catch (e) {
          // ignore stream errors
        }

        // Set the final result with proper formatting
        const finalResponse = 
          (accumulatedThoughts ? `<think>${accumulatedThoughts}</think>` : '') + 
          (finalResult ? `\n${finalResult}` : '');
          
        result = { content: finalResponse };
        
        // Update the required outputs.result.output structure for workflow compatibility
        props.data.outputs = {
          ...props.data.outputs,
          result: {
            output: finalResponse
          }
        };
        
        // Now that the agent has completed, trigger the next node in the workflow
        if (props.vueFlowInstance) {
          const { getEdges, findNode } = props.vueFlowInstance;
          const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
          const responseNode = responseNodeId ? findNode(responseNodeId) : null;
          
          if (responseNode) {
            responseNode.data.inputs.response = finalResponse;
          }

          return result;
        }
      } else {
        // --- Regular API call (non-agent mode) ---
        const requestBody = {
          model: props.data.inputs.model,
          messages: [
            {
              role: "system",
              content: props.data.inputs.system_prompt
            },
            {
              role: "user",
              content: finalPrompt
            }
          ],
          max_tokens: props.data.inputs.max_completion_tokens || 1000,
          temperature: props.data.inputs.temperature || 0.7
        };

        const response = await fetch(props.data.inputs.endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${props.data.inputs.api_key}`
          },
          body: JSON.stringify(requestBody)
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`API error (${response.status}): ${errorText}`);
        }

        const responseData = await response.json();
        const responseText = responseData.choices && responseData.choices[0]?.message?.content;
        
        // Update the outputs with the result
        props.data.outputs = {
          ...props.data.outputs,
          response: responseText,
          result: {
            output: responseText
          }
        };

        // Update connected response nodes
        if (props.vueFlowInstance) {
          const { getEdges, findNode } = props.vueFlowInstance;
          const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
          const responseNode = responseNodeId ? findNode(responseNodeId) : null;
          
          if (responseNode) {
            responseNode.data.inputs.response = responseText;
          }
        }

        result = { content: responseText };
        return result;
      }

      // Handle error in result (this is now only for agent mode since we return earlier for regular API calls)
      if (result && result.error) {
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
    agentMaxSteps,
    
    // Methods
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    sendToCodeEditor
  }
}