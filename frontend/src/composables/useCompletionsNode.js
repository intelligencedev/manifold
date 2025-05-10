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
  const isHovered = ref(false)
  const customStyle = ref({
    width: '380px',
    height: '760px'
  })
  
  // Agent mode is always enabled - hardcoded to true
  const agentMode = ref(true)
  
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
  
  // Computed properties for form binding
  // Hardcode endpoint to the agent endpoint
  const endpoint = computed({
    get: () => '/api/agents/react',
    set: () => { /* no-op - endpoint is fixed */ }
  })
  
  const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value },
  })
  
  const user_prompt = computed({
    get: () => props.data.inputs.user_prompt,
    set: (value) => { props.data.inputs.user_prompt = value },
  })
  
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
          model: ''  // Using default model from the backend
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
      }

      return result;

      // Handle error in result
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
    // Ensure endpoint is set to agent endpoint
    props.data.inputs.endpoint = '/api/agents/react';
  })
  
  // Use config for default key
  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig && newConfig.Completions) {
        if (!props.data.inputs.api_key) {
          props.data.inputs.api_key = newConfig.Completions.APIKey;
        }
      }
    },
    { immediate: true }
  )
  
  return {
    // State
    showApiKey,
    isHovered,
    
    // Computed properties
    endpoint,
    api_key,
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