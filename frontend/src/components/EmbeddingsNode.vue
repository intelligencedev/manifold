<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container embeddings-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

        <!-- Embeddings Endpoint Input -->
        <div class="input-field">
            <label class="input-label">Endpoint:</label>
            <input type="text" class="input-text" v-model="embeddings_endpoint" />
        </div>

        <!-- Input/Output Handles -->
        <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" />
        <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" position="right" />

        <!-- NodeResizer -->
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :min-width="200" :min-height="120" :width="200" :height="120" :node-id="props.id" @resize="onResize" />
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
const { getEdges, findNode, updateNodeData } = useVueFlow()

const emit = defineEmits(['update:data', 'resize'])

// The run() logic
onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

async function callEmbeddingsAPI(embeddingsNode, text) {
    // remove leading/trailing whitespace
    text = text.trim();

    const endpoint = embeddingsNode.data.inputs.embeddings_endpoint;
    const response = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({
            input: [text],
            // key is required but the value is ignored by our backend for now since we start it with a model already configured
            model: "nomic-embed-text-v1.5.Q8_0",
            encoding_format: "float"
        }),
    });

    if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`API error (${response.status}): ${errorText}`);
    }

    const responseData = await response.json();

    console.log('Embeddings API response:', responseData);

    return responseData;
}

async function run() {
    console.log('Running EmbeddingsNode:', props.id);

    try {
        const embeddingsNode = findNode(props.id);
        let inputText = "";

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
                    inputText += `\n\n${sourceNode.data.outputs.result.output}`;
                }
            }
        }

        inputText = inputText.trim();
        inputText = inputText.split(" ").slice(0, 512).join(" ")
        console.log('Processed input text:', inputText);

        const embeddingsData = await callEmbeddingsAPI(embeddingsNode, inputText);

        // convert embeddingsData to a string
        let embeddingsJson = JSON.stringify(embeddingsData, null, 2);

        console.log('Embeddings data:', embeddingsJson);

        // Update the node's output with the embeddings data
        props.data.outputs = {
            result: { output: embeddingsJson },
        };

        updateNodeData();

        return { embeddingsData };
    } catch (error) {
        console.error('Error in EmbeddingsNode run:', error);
        return { error };
    }
}

const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'Embeddings_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'EmbeddingsNode',
            labelStyle: { fontWeight: 'normal' },
            hasInputs: true,
            hasOutputs: true,
            inputs: {
                embeddings_endpoint: 'http://<llama.cpp endpoint only>/v1/embeddings',
            },
            outputs: {
                result: { output: '' },
            },
            style: {
                border: '1px solid #666',
                borderRadius: '4px',
                backgroundColor: '#333',
                color: '#eee',
                width: '200px',
                height: '120px',
            },
        }),
    },
})

const embeddings_endpoint = computed({
    get: () => props.data.inputs.embeddings_endpoint,
    set: (value) => { props.data.inputs.embeddings_endpoint = value },
})

const isHovered = ref(false)
const customStyle = ref({})

// Show/hide the handles
const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
}))

function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
}
</script>

<style scoped>
.embeddings-node {
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
    margin-bottom: 8px;
}

.input-text {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
}
</style>