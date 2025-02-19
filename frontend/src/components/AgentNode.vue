<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container agent-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

        <!-- Provider Selection -->
        <div class="input-field">
            <label for="provider-select" class="input-label">Provider:</label>
            <select id="provider-select" v-model="provider" class="input-select">
                <option value="llama-server">llama-server</option>
                <option value="mlx_lm.server">mlx_lm.server</option>
                <option value="openai">openai</option>
            </select>
        </div>

        <!-- Parameters Accordion -->
        <details class="parameters-accordion" open>
            <summary>Parameters</summary>

            <!-- Endpoint Input -->
            <div class="input-field">
                <label class="input-label">Endpoint:</label>
                <input type="text" class="input-text" v-model="endpoint" />
            </div>

            <!-- OpenAI API Key Input -->
            <div class="input-field">
                <label :for="`${data.id}-api_key`" class="input-label">OpenAI API Key:</label>
                <input :id="`${data.id}-api_key`" :type="showApiKey ? 'text' : 'password'" class="input-text"
                    v-model="api_key" />
                <button @click="showApiKey = !showApiKey" class="toggle-password">
                    <span v-if="showApiKey">👁️</span>
                    <span v-else>🙈</span>
                </button>
            </div>

            <!-- Model Selection -->
            <div class="input-field">
                <label :for="`${data.id}-model`" class="input-label">Model:</label>
                <input type="text" :id="`${data.id}-model`" v-model="model" class="input-text" />
            </div>

            <!-- Max Completion Tokens Input -->
            <div class="input-field">
                <label :for="`${data.id}-max_completion_tokens`" class="input-label">Max Completion Tokens:</label>
                <input type="number" :id="`${data.id}-max_completion_tokens`" v-model.number="max_completion_tokens"
                    class="input-text" min="1" />
            </div>

            <!-- Temperature Input -->
            <div class="input-field">
                <label :for="`${data.id}-temperature`" class="input-label">Temperature:</label>
                <input type="number" :id="`${data.id}-temperature`" v-model.number="temperature" class="input-text"
                    step="0.1" min="0" max="2" />
            </div>

            <!-- System Prompt (moved inside accordion) -->
            <div class="input-field">
                <label :for="`${data.id}-system_prompt`" class="input-label">System Prompt:</label>
                <textarea :id="`${data.id}-system_prompt`" v-model="system_prompt" class="input-textarea"></textarea>
            </div>
        </details>

        <!-- User Prompt -->
        <div class="input-field user-prompt-field">
            <label :for="`${data.id}-user_prompt`" class="input-label">User Prompt:</label>
            <textarea :id="`${data.id}-user_prompt`" v-model="user_prompt" class="input-textarea user-prompt-area"
                @mouseenter="handleTextareaMouseEnter" @mouseleave="handleTextareaMouseLeave"></textarea>
        </div>


        <!-- Input/Output Handles -->
        <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" />
        <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" position="right" />

        <!-- NodeResizer -->
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :width="360" :height="760" :min-width="360" :min-height="760" :node-id="props.id" @resize="onResize" />
    </div>
</template>

<script setup>
import { useConfigStore } from '@/stores/configStore'
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'Agent_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'AgentNode',
            labelStyle: { fontWeight: 'normal' },
            hasInputs: true,
            hasOutputs: true,
            inputs: {
                endpoint: '',
                api_key: "", // Get the API key from the config store
                // For now we provision the backend with a model already loaded. We need to add support for swapping it via this control.
                // Otherwise, this works with OpenAI models and needs to have a valid model name when using OpenAI.
                model: 'local',
                system_prompt: 'You are a helpful assistant.',
                user_prompt: 'Write a haiku about manifolds.',
                max_completion_tokens: 8192,
                temperature: 0.6,
            },
            outputs: { response: '' },
            models: ['local', 'chatgpt-4o-latest', 'gpt-4o', 'gpt-4o-mini', 'o1-mini', 'o1', 'o3-mini'],
            style: {
                border: '1px solid #666',
                borderRadius: '4px',
                backgroundColor: '#333',
                color: '#eee',
                width: '320px',
                height: '760px',
            },
        }),
    },
})

const configStore = useConfigStore()

const { getEdges, findNode, zoomIn, zoomOut, updateNodeData } = useVueFlow()

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

watch(() => configStore.config.Completions.Provider, (newProvider) => {
    if (newProvider) {
        props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
}, { immediate: true })

const showApiKey = ref(false);

// The run() logic
onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

// Watch for when the config is loaded and then set the Completions DefaultHost and APIKey
watch(
  () => configStore.config, 
  (newConfig) => {
    if (newConfig && newConfig.Completions) {
      if (!props.data.inputs.api_key) {
        props.data.inputs.api_key = newConfig.Completions.APIKey
      }
      if (!props.data.inputs.endpoint) {
        props.data.inputs.endpoint = newConfig.Completions.DefaultHost
      }
    }
  },
  { immediate: true }
)

async function callCompletionsAPI(agentNode, prompt) {
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;

    // Construct the endpoint URL
    let endpoint = agentNode.data.inputs.endpoint;

    // Helper to detect if the current model is an o1 model.
    const isO1Model = (model) => model.toLowerCase().startsWith("o1" || "o3");

    // Build the messages array according to the model type.
    let messages = [];
    let body = {};
    if (isO1Model(agentNode.data.inputs.model)) {
        body = {
            model: agentNode.data.inputs.model,
            max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
            temperature: agentNode.data.inputs.temperature,
            messages: [
                {
                    role: "user",
                    content: `${agentNode.data.inputs.system_prompt}\n\n${prompt}`,
                },
            ],
            stream: true,
        };
    } else {
        body = {
            model: agentNode.data.inputs.model,
            max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
            temperature: agentNode.data.inputs.temperature,
            messages: [
                {
                    role: "system",
                    content: agentNode.data.inputs.system_prompt,
                },
                {
                    role: "user",
                    content: prompt,
                },
            ],
            stream: true,
        };
    }

    const response = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
        },
        body: JSON.stringify(body),
    });

    if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`API error (${response.status}): ${errorText}`);
    }

    const reader = response.body.getReader();
    let buffer = "";
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = new TextDecoder().decode(value);
        buffer += chunk;

        let start = 0;
        for (let i = 0; i < buffer.length; i++) {
            if (buffer[i] === "\n") {
                const line = buffer.substring(start, i).trim();
                start = i + 1;

                if (line.startsWith("data: ")) {
                    const jsonData = line.substring(6);
                    if (jsonData === "[DONE]") break;
                    try {
                        const parsedData = JSON.parse(jsonData);
                        const delta = parsedData.choices[0]?.delta || {};

                        // For older models delta.content will have the token.
                        // For new "thinking" models, delta.thinking may be present.
                        const tokenContent = (delta.content || '') + (delta.thinking || '');

                        // Update outputs response
                        props.data.outputs.response += tokenContent;

                        if (responseNode) {
                            responseNode.data.inputs.response += tokenContent;
                            responseNode.run;
                        }
                    } catch (e) {
                        console.error("Error parsing response chunk:", e);
                    }
                }
            }
        }
        buffer = buffer.substring(start);
    }

    return { response };
}

async function run() {
    console.log('Running AgentNode:', props.id);

    try {
        const agentNode = findNode(props.id);
        let finalPrompt = props.data.inputs.user_prompt;

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

        return await callCompletionsAPI(agentNode, finalPrompt);
    } catch (error) {
        console.error('Error in AgentNode run:', error);
        return { error };
    }
}

const model = computed({
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
const endpoint = computed({
    get: () => props.data.inputs.endpoint,
    set: (value) => { props.data.inputs.endpoint = value },
})
const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value },
})
/** 
 * Computed Property for Max Completion Tokens
 */
const max_completion_tokens = computed({
    get: () => props.data.inputs.max_completion_tokens,
    set: (value) => { props.data.inputs.max_completion_tokens = value },
})
/** End of Computed Property **/

/** 
 * Computed Property for Temperature 
 */
const temperature = computed({
    get: () => props.data.inputs.temperature,
    set: (value) => { props.data.inputs.temperature = value },
})

const isHovered = ref(false)
const customStyle = ref({
    width: '320px',
    height: '760px'
})

// Show/hide the handles
const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
}))

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

const provider = computed({
    get: () => {
        if (props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions') {
            return 'openai';
        } else if (props.data.inputs.endpoint === configStore.config.Completions.DefaultHost) {
            // Check if the endpoint matches the provider type in the config
            if (configStore.config.Completions.Provider === 'llama-server') {
                return 'llama-server';
            } else if (configStore.config.Completions.Provider === 'mlx_lm.server') {
                return 'mlx_lm.server';
            }
        }
        // Default to llama-server if no match
        return 'llama-server';
    },
    set: (value) => {
        if (value === 'openai') {
            props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
        } else {
            props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
        }
    }
});
</script>

<style scoped>
.agent-node {
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
    margin-top: 5px;
    margin-bottom: 5px;
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
    width: 100%;
    /* Fill the container's horizontal space */
    resize: none;
    /* Prevent user dragging the bottom-right handle inside the textarea */
    overflow-y: auto;
    /* Scroll if the text is bigger than the area */
    min-height: 0;
    /* Prevent flex sizing issues (needed in some browsers) */
}

/* Basic styling for the parameters accordion */
.parameters-accordion {
    min-width: 180px !important;
    margin-bottom: 5px;
    margin-top: 5px;
    border: 1px solid #666;
    border-radius: 4px;
    background-color: #444;
    color: #eee;
    padding: 5px;
    width: 100%;
    box-sizing: border-box;
}

.parameters-accordion summary {
    cursor: pointer;
    padding: 5px;
    font-weight: bold;
}
</style>