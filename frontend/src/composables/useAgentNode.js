import { ref, computed, watch, onMounted } from 'vue'
import { useConfigStore } from '@/stores/configStore'
import { useCodeEditor } from './useCodeEditor'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'
import { useSystemPromptOptions } from './systemPrompts'

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
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize
  } = useNodeBase(props, emit)
  const { systemPromptOptions, systemPromptOptionsList } = useSystemPromptOptions()

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
  
  // Function to update response in real-time as stream content comes in
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
      } else if (!props.data.inputs.endpoint || props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions') {
        // If current endpoint is empty or OpenAI, set to default local
        props.data.inputs.endpoint = configStore.config?.Completions?.DefaultHost || 'http://localhost:32186/v1/chat/completions';
      }
      // Otherwise, keep user's custom endpoint
    }
  });
  

  
  // Node functionality
  async function run() {
    console.log('Running AgentNode:', props.id);
    props.data.outputs.error = null;
    props.data.outputs.response = ''; 
    onResponseUpdate('', ''); // Clear connected nodes too

    let result = { content: '' };

    try {
      let finalPrompt = props.data.inputs.user_prompt;

      // --- aggregate text from all connected source nodes ---
      const incomingEdges = getEdges.value.filter(edge => edge.target === props.id);
      for (const edge of incomingEdges) {
        const sourceNode = findNode(edge.source);
        if (sourceNode?.data?.outputs?.result?.output) {
          finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
        }
      }

      // --- Regular API call with streaming ---
      let visionContent = null;
      const imageDataUrls = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => findNode(edge.source))
        .filter(node => node?.data?.isImage && node.data.outputs?.result?.dataUrl)
        .map(node => node.data.outputs.result.dataUrl);

      if (imageDataUrls.length) {
        visionContent = [{ type: 'text', text: finalPrompt }];
        imageDataUrls.forEach(url => {
          visionContent.push({ type: 'image_url', image_url: { url } });
        });
      }

      let requestBody = {
        messages: [
          { role: 'system', content: props.data.inputs.system_prompt },
          { role: 'user', content: visionContent ? visionContent : finalPrompt }
        ],
        temperature: props.data.inputs.temperature ?? 0.7,
      };

      const modelName = props.data.inputs.model.toLowerCase();
      const currentProvider = provider.value;
      
      // Only include model parameter for OpenAI provider
      if (currentProvider === 'openai') {
        requestBody.model = props.data.inputs.model;
        
        if (modelName.startsWith('o') && /^o[0-9]/.test(modelName)) {
          requestBody.max_completion_tokens = props.data.inputs.max_completion_tokens || 1000;
          requestBody.reasoning_effort = 'high';
        } else {
          requestBody.max_completion_tokens = props.data.inputs.max_completion_tokens || 1000;
        }
      } else {
        // For non-OpenAI providers
        requestBody.max_completion_tokens = props.data.inputs.max_completion_tokens || 1000;
      }

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

            if (value.trim() === '[DONE]') { 
              await reader.cancel(); 
              break;
            }
            
            try {
              const chunkData = JSON.parse(value);
              let deltaContent = '';
              if (chunkData.choices && chunkData.choices[0] && chunkData.choices[0].delta) {
                deltaContent = chunkData.choices[0].delta.content || '';
              }
              
              if (deltaContent) {
                accumulatedContent += deltaContent;
                onResponseUpdate(accumulatedContent, accumulatedContent);
              }
            } catch (e) {
              console.warn('Failed to parse stream chunk as JSON:', value, e.message);
            }
          }
        } catch (e) {
           console.error('Error reading chat completion stream:', e.message);
           throw e; 
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
            'Authorization': `Bearer ${props.data.inputs.api_key}` 
          },
          body: JSON.stringify(requestBody) 
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
        onResponseUpdate(responseText, responseText); 
        result = { content: responseText };
      }
      return result;

    } catch (error) {
      console.error('Error in AgentNode run:', props.id, error);
      const errorMessage = error.message || "Unknown error occurred";
      
      props.data.outputs.error = errorMessage;
      const errorResponse = JSON.stringify({ 
        error: errorMessage,
        details: error.cause ? String(error.cause) : undefined,
        partialResponse: props.data.outputs.response 
      }, null, 2);
      props.data.outputs.response = errorResponse;
      
      const targetEdges = getEdges.value.filter(edge => edge.source === props.id);
      targetEdges.forEach(edge => {
          const connectedNode = findNode(edge.target);
          if (connectedNode && connectedNode.data && connectedNode.data.inputs) {
              connectedNode.data.inputs.response = errorResponse;
          }
      });
      return { error: errorMessage, content: props.data.outputs.response }; 
    }
  }
  
  /**
   * Sends code to the code editor
   */
  function sendToCodeEditor() {
    if (props.data.outputs && props.data.outputs.response) {
      const responseText = props.data.outputs.response;
      const codeBlockRegex = /```(?:javascript|js|python|go|typescript|ts|html|css|json|yaml|sh|bash)?\s*([\s\S]*?)```/gi;
      let allCode = "";
      let match;
      while((match = codeBlockRegex.exec(responseText)) !== null) {
        allCode += match[1].trim() + "\n\n";
      }

      if (allCode.trim()) {
        setEditorCode(allCode.trim());
      } else {
        setEditorCode(responseText); 
      }
    }
  }

  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    emit('resize', { id: props.id, width: event.width, height: event.height });
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
    if (props.data.style) {
        customStyle.value.width = props.data.style.width || '380px';
        customStyle.value.height = props.data.style.height || '906px'; 
    }
  })
  
  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig && newConfig.Completions) {
        if (!props.data.inputs.api_key && newConfig.Completions.APIKey) {
          props.data.inputs.api_key = newConfig.Completions.APIKey;
        }
        if (!props.data.inputs.endpoint && newConfig.Completions.DefaultHost) {
          props.data.inputs.endpoint = newConfig.Completions.DefaultHost;
        }
      }
    },
    { immediate: true, deep: true }
  )
  
  watch(() => configStore.config?.Completions?.Provider, (newProvider) => {
    if (newProvider && provider.value !== 'openai') { 
      props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
  }, { immediate: true });
  
  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  });

  if (!props.data.style) {
    props.data.style = {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '380px',
        height: '906px', 
    };
  }
  customStyle.value.width = props.data.style.width || '380px';
  customStyle.value.height = props.data.style.height || '906px';
  
  return {
    showApiKey,
    enableToolCalls,
    agentMode: ref(false), // Keep this ref but set to false always
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
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    sendToCodeEditor
  }
}