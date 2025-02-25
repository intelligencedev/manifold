<template>
    <div :style="data.style" class="node-container tool-node">
        <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

        <!-- Endpoint Input -->
        <div class="input-field">
            <label :for="`${data.id}-endpoint`" class="input-label">Endpoint:</label>
            <input :id="`${data.id}-endpoint`" type="text" v-model="endpoint" @change="updateNodeData"
                class="input-text" />
        </div>

        <!-- Text Input -->
        <div class="input-field">
            <label :for="`${data.id}-text`" class="input-label">Text:</label>
            <textarea :id="`${data.id}-text`" v-model="text" @change="updateNodeData" class="input-textarea"></textarea>
        </div>

        <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
        <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />
    </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core';
import { watch, ref, computed, onMounted } from 'vue';

const { getEdges, findNode, onConnect, onConnectStart, onConnectEnd, addEdges } = useVueFlow();

const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'TextSplitter_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'textSplitterNode',
            labelStyle: {},
            style: {},
            inputs: {
                endpoint: 'http://localhost:8080/api/split-text',
                text: '',
            },
            outputs: {},
            hasInputs: true,
            hasOutputs: true,
            inputHandleColor: '#777',
            outputHandleShape: '50%',
            handleColor: '#777',
        }),
    },
});

const endpoint = ref(props.data.inputs?.endpoint || 'http://localhost:8080/api/split-text');
const text = ref(props.data.inputs?.text || '');
const outputConnectionCount = ref(0); // Initialize output connection count

watch(
    () => props.data,
    (newData) => {
        endpoint.value = newData.inputs?.endpoint || 'http://localhost:8080/api/split-text';
        text.value = newData.inputs?.text || '';
        emit('update:data', { id: props.id, data: newData });
    },
    { deep: true }
);

onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

// Watch for changes in edges and update output connections accordingly
watch(
    () => getEdges.value,
    () => {
        updateOutputConnections();
    },
    { deep: true }
);

const updateOutputConnections = async () => {
    // Find connected output edges
    const outputEdges = getEdges.value.filter((edge) => edge.source === props.id);
    outputConnectionCount.value = outputEdges.length;

    // Update the node data with the new output connection count
    updateNodeData();
};

const updateNodeData = async () => {
    const updatedData = {
        ...props.data,
        inputs: {
            endpoint: endpoint.value,
            text: text.value,
        },
        outputs: {}, // Initialize outputs as an empty object
        num_chunks: outputConnectionCount.value
    };

    // Get connected input edges and their source nodes
    const inputEdges = getEdges.value.filter((edge) => edge.target === props.id && edge.targetHandle === 'input');
    for (const edge of inputEdges) {
        const sourceNode = findNode(edge.source);
        if (sourceNode && sourceNode.data.outputs) {
            // Assuming the text to be split is in the 'output' property of the source node's outputs
            if (sourceNode.data.outputs[edge.sourceHandle]) {
                updatedData.inputs.text = sourceNode.data.outputs[edge.sourceHandle];
            }
        }
    }
    emit('update:data', { id: props.id, data: updatedData });
};

async function run() {
    console.log('Running TextSplitterNode:', props.id);

    const connectedTargetEdges = getEdges.value.filter(
        (edge) => edge.target === props.id
    );

    // Get the first connected edge
    const targetEdge = connectedTargetEdges[0];

    console.log('Connected target edge:', targetEdge);

    // Get the source node of the connected edge
    const sourceNode = findNode(targetEdge.source);

    console.log('Source node:', sourceNode);

    // Get the response value from the source node's outputs
    const response = sourceNode.data.outputs.result.output;

    console.log('Response:', response);

    // Update the input text with the response value
    text.value = response;

    // Update the node data with the new input text
    updateNodeData();

    // Get the source edges
    const sourceEdges = getEdges.value.filter(
        (edge) => edge.source === props.id
    );

    // Get the target nodes of the source edges
    const targetNodes = sourceEdges.map((edge) => findNode(edge.target));

    console.log('Target nodes:', targetNodes);

    // Split the text by the number of target nodes, for example if there are two target nodes, then the text should be split into two strings:
    // "This is my example test" -> ["This is my", "example test"]
    
    // First get a count of the source connections
    const sourceCount = sourceEdges.length;

    // Split the text into chunks based on the number of source connections
    const words = text.value.split(' ');
    const wordsPerChunk = Math.ceil(words.length / sourceCount);
    const chunks = [];

    for (let i = 0; i < sourceCount; i++) {
        const start = i * wordsPerChunk;
        const end = Math.min(start + wordsPerChunk, words.length);
        chunks.push(words.slice(start, end).join(' '));

        // Update the outputs for each target node
        if (targetNodes[i]) {
            console.log('Updating target node:', targetNodes[i]);
            targetNodes[i].data.inputs.response += chunks[i];
            targetNodes[i].data.inputs.text += chunks[i];

            updateNodeData();
        }
    }


    console.log('Chunks:', chunks);

}

const emit = defineEmits(['update:data']);

</script>

<style scoped>
/* Same styles as other tool nodes */
.node-container {
    border: 3px solid var(--node-border-color) !important;
    background-color: var(--node-bg-color) !important;
    box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
    padding: 15px;
    border-radius: 8px;
    color: var(--node-text-color);
    font-family: 'Roboto', sans-serif;
}

.tool-node {
    --node-border-color: #777 !important;
    --node-bg-color: #1e1e1e !important;
    --node-text-color: #eee;
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

.input-textarea {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
    min-height: 60px;
}

.handle-input,
.handle-output {
    width: 12px;
    height: 12px;
    border: none;
    background-color: #777;
}
</style>