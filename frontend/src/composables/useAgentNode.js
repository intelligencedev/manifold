import { ref, computed, watch, onMounted } from 'vue'
import { useConfigStore } from '@/stores/configStore'
import { useCodeEditor } from './useCodeEditor'
import { useVueFlow } from '@vue-flow/core'

/**
 * Composable for managing AgentNode state and functionality
 */
export function useAgentNode(props, emit) {
  // Access VueFlow API for node updates
  const { getEdges, findNode, updateNodeData } = useVueFlow();
  const configStore = useConfigStore()
  const { setEditorCode } = useCodeEditor()
  
  // State variables
  const showApiKey = ref(false)
  const enableToolCalls = ref(false)
  const agentMode = ref(false)
  const isHovered = ref(false)
  const customStyle = ref({
    width: '380px',
    height: '760px' // Default height, matches NodeResizer minHeight
  })
  
  // Store the original endpoint when toggling agent mode
  const originalEndpoint = ref('')
  
  // Computed property for agent max steps
  const agentMaxSteps = computed(() => 
    configStore.config?.Completions?.Agent?.MaxSteps || 30
  )

  // Helper function to create an event stream splitter
  // This is suitable for SSE format where events are `data: <payload>\n\n`
  function createEventStreamSplitter() {
    let buffer = '';
    return new TransformStream({
      transform(chunk, controller) {
        buffer += chunk;
        let idx;
        while ((idx = buffer.indexOf("\n\n")) !== -1) {
          const event = buffer.slice(0, idx).replace(/^data:\s*/gm,'').trim();
          if (event) { // Ensure non-empty event after processing
            controller.enqueue(event);
          }
          buffer = buffer.slice(idx + 2);
        }
      },
      flush(controller) {
        // Handle any remaining data in the buffer when the stream closes
        if (buffer.trim()) {
          const event = buffer.replace(/^data:\s*/gm,'').trim();
          if (event) {
            controller.enqueue(event);
          }
        }
      }
    });
  }
  
  // Function to update response in real-time as agent thoughts or stream content comes in
  function onResponseUpdate(content, fullResponse) {
    // Update the UI with the streamed response
    props.data.outputs = {
      ...props.data.outputs,
      response: content,
      result: { // Keep result.output consistent with the latest full content
        output: fullResponse 
      }
    };
    
    // Also propagate updates to connected ResponseNode components
    const edges = getEdges.value.filter(edge => edge.source === props.id)
    edges.forEach(edge => {
      const targetId = edge.target
      const node = findNode(targetId)
      if (node && node.data?.inputs) {
        const updated = {
          ...node.data,
          inputs: { ...node.data.inputs, response: content }
        }
        updateNodeData(targetId, updated)
      }
    })
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
      
      if (props.data.inputs.endpoint?.includes('localhost') ||
          props.data.inputs.endpoint?.includes('127.0.0.1')) {
        if (props.data._lastLocalProvider === 'mlx_lm.server') {
          return 'mlx_lm.server';
        }
        return 'llama-server'; // Default local to llama-server
      }
      // Default to openai if endpoint looks like openai, otherwise llama-server as a general default
      return props.data.inputs.endpoint?.includes('openai.com') ? 'openai' : 'llama-server';
    },
    set: (value) => {
      if (value !== 'openai') {
        props.data._lastLocalProvider = value; // Store for custom local endpoints
      }
      
      if (value === 'openai') {
        props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
      } else if (!props.data.inputs.endpoint || props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions' || props.data.inputs.endpoint.startsWith('/api/agents/react')) {
        // If current endpoint is empty, OpenAI, or agent endpoint, set to default local
        props.data.inputs.endpoint = configStore.config?.Completions?.DefaultHost || 'http://localhost:32186/v1/chat/completions';
      }
      // Otherwise, keep user's custom endpoint if it's not OpenAI or agent
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
    width: '100%', // Ensure it fills the resizer
    height: '100%' // Ensure it fills the resizer
  }))
  
  // Node functionality
  async function run() {
    console.log('Running AgentNode:', props.id);
    // Clear previous error/response before new run
    props.data.outputs.error = null;
    props.data.outputs.response = ''; 
    onResponseUpdate('', ''); // Clear connected nodes too

    let result = { content: '' }; // Initialize result

    try {
      let finalPrompt = props.data.inputs.user_prompt;
      
      if (props.vueFlowInstance) {
        const { getEdges, findNode } = props.vueFlowInstance;
        const connectedSources = getEdges.value
          .filter((edge) => edge.target === props.id)
          .map((edge) => edge.source);
  
        for (const sourceId of connectedSources) {
          const sourceNode = findNode(sourceId);
          if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result && sourceNode.data.outputs.result.output) {
            finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
          }
        }
      }
      
      if (agentMode.value) {
        // --- ReAct agent call with streaming ---
        const sseResp = await fetch(props.data.inputs.endpoint, { // Endpoint is now /api/agents/react/stream
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Accept': 'text/event-stream'
          },
          body: JSON.stringify({
            objective: finalPrompt,
            max_steps: agentMaxSteps.value,
            model: provider.value === 'openai' ? props.data.inputs.model : '' // Pass model if OpenAI for agent
          })
        });
        
        if (!sseResp.ok) {
          const errorText = await sseResp.text();
          throw new Error(`Agent API error (${sseResp.status}): ${errorText}`);
        }

        const reader = sseResp.body
              .pipeThrough(new TextDecoderStream())
              .pipeThrough(createEventStreamSplitter())
              .getReader();

        let accumulatedThoughts = '';
        let finalAnswer = ''; // Renamed from finalResult for clarity

        try {
          while (true) {
            const { value, done } = await reader.read();
            if (done) break;
            if (value === '[[EOF]]') { // Agent specific EOF
              await reader.cancel();
              break;
            }
            
            const thinkMatch = value.match(/<think>([\s\S]*?)<\/think>/);
            if (thinkMatch && thinkMatch[1]) {
              accumulatedThoughts += thinkMatch[1].trim() + '\n';
            } else if (value.trim()) { // Avoid adding empty lines if value is just whitespace
              finalAnswer = value; // Assuming final answer comes after thoughts
            }
            const combinedResponse = 
              (accumulatedThoughts ? `<think>${accumulatedThoughts.trim()}</think>` : '') + 
              (finalAnswer ? `\n${finalAnswer}` : '');
            onResponseUpdate(combinedResponse, combinedResponse);
          }
        } catch (e) {
          console.warn('Agent stream reading ended potentially due to cancellation or minor error:', e.message);
        }

        const finalResponseText = 
          (accumulatedThoughts ? `<think>${accumulatedThoughts.trim()}</think>` : '') + 
          (finalAnswer ? `\n${finalAnswer}` : '');
        
        props.data.outputs = {
          ...props.data.outputs,
          response: finalResponseText,
          result: { output: finalResponseText }
        };
        result = { content: finalResponseText };

      } else {
        // --- Regular API call (non-agent mode) with streaming ---
        let visionContent = null;
        if (props.vueFlowInstance) {
          const { getEdges, findNode } = props.vueFlowInstance;
          const imageSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => findNode(edge.source))
            .filter((node) => node?.data?.isImage && node.data.outputs?.result?.dataUrl);
          if (imageSources.length > 0) {
            visionContent = [{ type: 'text', text: finalPrompt }]; // Use finalPrompt for text part
            imageSources.forEach((node) => {
              visionContent.push({
                type: 'image_url',
                image_url: { url: node.data.outputs.result.dataUrl }
              });
            });
          }
        }

        let requestBody = {
          model: props.data.inputs.model,
          messages: [
            { role: 'system', content: props.data.inputs.system_prompt },
            { role: 'user', content: visionContent ? visionContent : finalPrompt }
          ],
          temperature: props.data.inputs.temperature !== undefined ? props.data.inputs.temperature : 0.7,
        };
        
        const modelName = props.data.inputs.model.toLowerCase();
        if (modelName.startsWith('o') && /^o[0-9]/.test(modelName)) {
          requestBody.max_completion_tokens = props.data.inputs.max_completion_tokens || 1000;
          requestBody.reasoning_effort = 'high';
        } else {
          requestBody.max_tokens = props.data.inputs.max_completion_tokens || 1000;
        }

        const currentProvider = provider.value;
        const canStream = currentProvider === 'openai' || currentProvider === 'llama-server' || currentProvider === 'mlx_lm.server';

        if (canStream) {
          requestBody.stream = true;
          
          const headers = {
            'Content-Type': 'application/json',
            'Accept': 'text/event-stream'
          };
          if (currentProvider === 'openai' && props.data.inputs.api_key) {
            headers['Authorization'] = `Bearer ${props.data.inputs.api_key}`;
          }

          const sseResp = await fetch(props.data.inputs.endpoint, {
            method: 'POST',
            headers: headers,
            body: JSON.stringify(requestBody)
          });

          if (!sseResp.ok) {
            const errorText = await sseResp.text();
            throw new Error(`API error (${sseResp.status}): ${errorText}`);
          }

          const reader = sseResp.body
              .pipeThrough(new TextDecoderStream())
              .pipeThrough(createEventStreamSplitter())
              .getReader();

          let accumulatedContent = '';
          try {
            while (true) {
              const { value, done } = await reader.read();
              if (done) break;

              if (value.trim() === '[DONE]') { // OpenAI stream termination
                await reader.cancel(); // Ensure reader is cancelled
                break;
              }
              
              try {
                const chunkData = JSON.parse(value);
                let deltaContent = '';
                if (chunkData.choices && chunkData.choices[0] && chunkData.choices[0].delta) {
                  deltaContent = chunkData.choices[0].delta.content || '';
                }
                // Add other potential chunk structures if needed for non-OpenAI compatible streams
                // else if (chunkData.token && chunkData.token.text) { // Example for another format
                //   deltaContent = chunkData.token.text;
                // }

                if (deltaContent) {
                  accumulatedContent += deltaContent;
                  onResponseUpdate(accumulatedContent, accumulatedContent);
                }
              } catch (e) {
                // If JSON.parse fails, it might be a non-JSON part of the stream or an error message.
                // For now, we log and ignore, assuming valid chunks are JSON.
                console.warn('Failed to parse stream chunk as JSON:', value, e.message);
              }
            }
          } catch (e) {
             console.error('Error reading chat completion stream:', e.message);
             // If stream breaks, accumulatedContent has partial data.
             // The main catch block will handle displaying this as an error context if needed.
             throw e; // Re-throw to be caught by the main try-catch
          }
          
          props.data.outputs = {
              ...props.data.outputs,
              response: accumulatedContent,
              result: { output: accumulatedContent }
          };
          result = { content: accumulatedContent };

        } else {
          // --- Fallback to non-streaming for other providers ---
          const response = await fetch(props.data.inputs.endpoint, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${props.data.inputs.api_key}` // Assuming non-streaming might still need key
            },
            body: JSON.stringify(requestBody) // requestBody already has max_tokens etc.
          });

          if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`API error (${response.status}): ${errorText}`);
          }

          const responseData = await response.json();
          const responseText = responseData.choices && responseData.choices[0]?.message?.content || '';
          
          props.data.outputs = {
            ...props.data.outputs,
            response: responseText,
            result: { output: responseText }
          };
          onResponseUpdate(responseText, responseText); // Ensure connected nodes get updated
          result = { content: responseText };
        }
      }
      return result;

    } catch (error) {
      console.error('Error in AgentNode run:', props.id, error);
      const errorMessage = error.message || "Unknown error occurred";
      
      props.data.outputs.error = errorMessage;
      // Display the error in the response area
      const errorResponse = JSON.stringify({ 
        error: errorMessage,
        details: error.cause ? String(error.cause) : undefined,
        // Include partial content if available (e.g. from a broken stream)
        partialResponse: props.data.outputs.response 
      }, null, 2);
      props.data.outputs.response = errorResponse;
      
      // Update connected response nodes with the error
      if (props.vueFlowInstance) {
        const { getEdges, findNode } = props.vueFlowInstance;
        const targetEdges = getEdges.value.filter(edge => edge.source === props.id);
        targetEdges.forEach(edge => {
            const connectedNode = findNode(edge.target);
            if (connectedNode && connectedNode.data && connectedNode.data.inputs) {
                connectedNode.data.inputs.response = errorResponse;
            }
        });
      }
      return { error: errorMessage, content: props.data.outputs.response }; // Return error object for workflow
    }
  }
  
  /**
   * Sends code to the code editor
   */
  function sendToCodeEditor() {
    if (props.data.outputs && props.data.outputs.response) {
      const responseText = props.data.outputs.response;
      // Try to extract from common markdown code blocks
      const codeBlockRegex = /```(?:javascript|js|python|go|typescript|ts|html|css|json|yaml|sh|bash)?\s*([\s\S]*?)```/gi;
      let allCode = "";
      let match;
      while((match = codeBlockRegex.exec(responseText)) !== null) {
        allCode += match[1].trim() + "\n\n";
      }

      if (allCode.trim()) {
        setEditorCode(allCode.trim());
      } else {
        // If no standard code blocks, check for <think> blocks or send the whole response
        const thinkBlockRegex = /<think>([\s\S]*?)<\/think>/gi;
        let thinkContent = "";
        while((match = thinkBlockRegex.exec(responseText)) !== null) {
          thinkContent += match[1].trim() + "\n\n";
        }
        if (thinkContent.trim()) {
           setEditorCode(thinkContent.trim());
        } else {
          setEditorCode(responseText); // Fallback to entire response
        }
      }
    }
  }

  // Event handlers
  function onResize(event) {
    // Update internal customStyle for immediate feedback if needed,
    // but primary source of truth for size should be props.data.style if persisted
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    
    // Emit event for parent/workflow to potentially save new dimensions
    emit('resize', { id: props.id, width: event.width, height: event.height });
    
    // Optionally update props.data.style directly if that's the convention
    // props.data.style.width = `${event.width}px`;
    // props.data.style.height = `${event.height}px`;
  }
  
  function handleTextareaMouseEnter() {
    emit('disable-zoom');
  }
  
  function handleTextareaMouseLeave() {
    emit('enable-zoom');
  }
  
  // Lifecycle hooks and watchers
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run;
    }
    // Ensure initial size from props.data.style is reflected in customStyle
    if (props.data.style) {
        customStyle.value.width = props.data.style.width || '380px';
        customStyle.value.height = props.data.style.height || '906px'; // Updated default to match template
    }
  })
  
  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig && newConfig.Completions) {
        if (!props.data.inputs.api_key && newConfig.Completions.APIKey) {
          props.data.inputs.api_key = newConfig.Completions.APIKey;
        }
        // Only set default endpoint if it's not already an agent endpoint or a user-defined one
        const isAgentEndpoint = props.data.inputs.endpoint && props.data.inputs.endpoint.startsWith('/api/agents/react');
        if (!props.data.inputs.endpoint && !isAgentEndpoint && newConfig.Completions.DefaultHost) {
          props.data.inputs.endpoint = newConfig.Completions.DefaultHost;
        }
      }
    },
    { immediate: true, deep: true }
  )
  
  watch(() => configStore.config?.Completions?.Provider, (newProvider) => {
    if (newProvider && provider.value !== 'openai' && !agentMode.value) { // Don't override if agent mode is on
      props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
  }, { immediate: true });
  
  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  });
  
  watch(agentMode, (on) => {
    if (on) {
      originalEndpoint.value = props.data.inputs.endpoint; // Save current endpoint
      props.data.inputs.endpoint = '/api/agents/react/stream'; // Set to agent stream endpoint
    } else {
      if (originalEndpoint.value && originalEndpoint.value !== '/api/agents/react/stream') {
        props.data.inputs.endpoint = originalEndpoint.value;
      } else {
        // Fallback to default based on current provider if originalEndpoint was agent or empty
        if (provider.value === 'openai') {
          props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
        } else {
          props.data.inputs.endpoint = configStore.config?.Completions?.DefaultHost || 'http://localhost:32186/v1/chat/completions';
        }
      }
      originalEndpoint.value = ''; // Clear stored original endpoint
    }
  });

  // Ensure props.data.style is initialized
  if (!props.data.style) {
    props.data.style = {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '380px',
        height: '906px', // Match NodeResizer minHeight
    };
  }
  // Ensure customStyle reflects initial props.data.style
  customStyle.value.width = props.data.style.width || '380px';
  customStyle.value.height = props.data.style.height || '906px';
  
  return {
    showApiKey,
    enableToolCalls,
    agentMode,
    selectedSystemPrompt,
    isHovered,
    systemPromptOptionsList,
    providerOptions,
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
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    sendToCodeEditor
  }
}