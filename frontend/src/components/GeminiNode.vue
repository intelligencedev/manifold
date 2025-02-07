<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container gemini-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

        <!-- Model Selection -->
        <div class="input-field">
            <label :for="`${data.id}-model`" class="input-label">Model:</label>
            <select :id="`${data.id}-model`" v-model="selectedModel" class="input-select">
                <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
            </select>
        </div>

        <!-- System Prompt (Not directly used by Gemini API, but can be included in the prompt) -->
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

        <!-- Google AI API Key Input -->
        <div class="input-field">
            <label :for="`${data.id}-api_key`" class="input-label">Google AI API Key:</label>
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
import { ref, computed, onMounted, nextTick } from 'vue'
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

async function callGeminiAPI(geminiNode, prompt) {
  const responseNodeId = getEdges.value.find(
    (e) => e.source === props.id
  )?.target;
  const responseNode = responseNodeId ? findNode(responseNodeId) : null;

  // Construct the request body
  const requestBody = {
    contents: [
      {
        role: "user",
        parts: [{ text: prompt }],
      },
    ],
  };

  const endpoint = `https://generativelanguage.googleapis.com/v1beta/models/${geminiNode.data.inputs.model}:streamGenerateContent?key=${geminiNode.data.inputs.api_key}`;

  const response = await fetch(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(requestBody),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`API error (${response.status}): ${errorText}`);
  }

  const reader = response.body.getReader();
  let buffer = "";
  let incompleteJsonResponse = ""; // Buffer for incomplete JSON
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const chunk = new TextDecoder().decode(value);
    buffer += chunk;
    console.log(buffer);

    // Process each complete JSON object
    let start = 0;
    while (true) {
      let jsonStart = buffer.indexOf("{", start);
      if (jsonStart === -1) break;

      incompleteJsonResponse += buffer.substring(start, jsonStart);
      buffer = buffer.substring(jsonStart);

      try {
        const { valid, completeObject } = parseIncompleteJson(buffer);
        if (valid) {
          const responseContent =
            completeObject.candidates?.[0]?.content?.parts?.[0]?.text || "";

          // Update outputs response incrementally
          props.data.outputs.response += responseContent;

          // Update connected response node incrementally
          if (responseNode) {
            responseNode.data.inputs.response += responseContent;
            await nextTick(); // Ensure UI updates
            responseNode.run?.();
          }
          buffer = buffer.substring(getCompleteJsonLength(buffer));
          incompleteJsonResponse = ""; // Reset incomplete JSON buffer
          start = 0; // Reset start position for next object
        } else {
          // If not a valid or complete JSON, move to the next character
          start = jsonStart + 1;
          break; // Exit while loop and wait for more data
        }
      } catch (e) {
        console.error("Error processing JSON:", e);
        break;
      }
    }
  }

  return { response };
}

function parseIncompleteJson(jsonString) {
  try {
    const validJson = JSON.parse(jsonString);
    return { valid: true, completeObject: validJson };
  } catch (e) {
    // Attempt to fix the JSON string
    let fixedJsonString = jsonString;
    if (e.message.includes("Unexpected end of JSON input")) {
      // Try to close any unclosed braces or brackets
      const openBraces = (fixedJsonString.match(/{/g) || []).length;
      const closeBraces = (fixedJsonString.match(/}/g) || []).length;
      const openBrackets = (fixedJsonString.match(/\[/g) || []).length;
      const closeBrackets = (fixedJsonString.match(/\]/g) || []).length;
      fixedJsonString += "}".repeat(openBraces - closeBraces);
      fixedJsonString += "]".repeat(openBrackets - closeBrackets);
    }
    try {
      const fixedJson = JSON.parse(fixedJsonString);
      return { valid: true, completeObject: fixedJson };
    } catch (e) {
      return { valid: false, completeObject: null };
    }
  }
}

function getCompleteJsonLength(jsonString) {
  let openBraces = 0;
  let openBrackets = 0;
  for (let i = 0; i < jsonString.length; i++) {
    if (jsonString[i] === "{") openBraces++;
    if (jsonString[i] === "}") openBraces--;
    if (jsonString[i] === "[") openBrackets++;
    if (jsonString[i] === "]") openBrackets--;
    if (openBraces === 0 && openBrackets === 0) {
      return i + 1;
    }
  }
  return jsonString.length; // Return total length if no complete object is found
}

async function run() {
    console.log('Running GeminiNode:', props.id);

    try {
        const geminiNode = findNode(props.id);

        // Clear previous outputs
        props.data.outputs.response = '';

        let finalPrompt = props.data.inputs.user_prompt;

        // Optionally include the system prompt in the user prompt for Gemini
        if (props.data.inputs.system_prompt) {
            finalPrompt = `${props.data.inputs.system_prompt}\n\n${finalPrompt}`;
        }

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

        return await callGeminiAPI(geminiNode, finalPrompt);
    } catch (error) {
        console.error('Error in GeminiNode run:', error);
        return { error };
    }
}

const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'Gemini_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'GeminiNode',
            labelStyle: { fontWeight: 'normal' },
            hasInputs: true,
            hasOutputs: true,
            inputs: {
                api_key: '',
                model: 'gemini-2.0-flash', // Default to a Gemini model
                system_prompt: '', // Optional system prompt
                user_prompt: 'Summarize the following text:',
            },
            outputs: { response: '' },
            models: ['gemini-2.0-flash', 'gemini-2.0-pro-exp-02-05' , 'gemini-2.0-flash-lite-preview-02-05', 'gemini-2.0-flash-thinking-exp-01-21'],
            // Match the same defaults as ResponseNode
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

const selectedModel = computed({
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
const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value },
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
.gemini-node {
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
</style>