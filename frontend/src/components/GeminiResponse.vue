<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container gemini-response-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

        <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
        <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

        <div class="header">
            <div class="controls">
                <div class="select-container">
                    <label for="render-mode">Render Mode:</label>
                    <select id="render-mode" v-model="selectedRenderMode">
                        <option value="markdown">Markdown</option>
                        <option value="raw">Raw Text</option>
                    </select>
                </div>

                <!-- Font Size Controls -->
                <div class="font-size-controls">
                    <button @click.prevent="decreaseFontSize">-</button>
                    <button @click.prevent="increaseFontSize">+</button>
                </div>

                <!-- Copy Button -->
                <button class="copy-button" @click="copyToClipboard" :disabled="isCopying">
                    Copy
                </button>
            </div>

            <!-- Optional: Feedback Message -->
            <div v-if="copyStatus" class="copy-feedback">
                {{ copyStatus }}
            </div>
        </div>

        <div class="text-container" ref="textContainer" @scroll="handleScroll" @mouseenter="$emit('disable-zoom')"
            @mouseleave="$emit('enable-zoom')" :style="{ fontSize: `${currentFontSize}px` }">
            <div v-if="selectedRenderMode === 'raw'" class="raw-text">
                {{ response }}
            </div>
            <div v-else-if="selectedRenderMode === 'markdown'" class="markdown-text" v-html="markdownToHtml"></div>
        </div>
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :min-width="350" :min-height="400" :node-id="props.id" @resize="onResize" />
    </div>
</template>

<script setup>
import { reactive, watch, ref, computed, nextTick, onMounted } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { marked } from "marked";
import { NodeResizer } from "@vue-flow/node-resizer";

const { getEdges, findNode, updateNodeData } = useVueFlow()

onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

async function run() {
    console.log("Running GeminiResponse:", props.id);

    // Get target edges
    const targetEdges = getEdges.value;

    // Get the source node
    const connectedSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => edge.source);

    if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0]);

        console.log("Source node:", sourceNode);

        if (sourceNode.data.outputs.response) {
            // Get the response from the source node
            props.data.inputs.response = sourceNode.data.outputs.response;

            // The GeminiNode looks for sourceNode.data.outputs.response
            // So, store your aggregated text (or whatever you want) in the same structure:
            props.data.outputs = {
                result: {
                    output: response.value, // or results, or both, depending on your preference
                },
            }

            updateNodeData();
        }
    }



    // The AgentNode looks for sourceNode.data.outputs.result.output
    // So, store your aggregated text (or whatever you want) in the same structure:
    props.data.outputs = {
        result: {
            output: response.value, // or results, or both, depending on your preference
        },
    }
}

// Define props and emits
const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'GeminiResponse_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'GeminiResponseNode',
            labelStyle: {
                fontWeight: 'normal',
            },
            hasInputs: true,
            hasOutputs: true,
            inputs: {
                response: ""
            },
            outputs: {
            },
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
const emit = defineEmits(["update:data", "disable-zoom", "enable-zoom", "resize"]);

// Reactive state for render mode
const selectedRenderMode = ref("raw");

// References to DOM elements
const textContainer = ref(null);

// Auto-scroll control
const isAutoScrollEnabled = ref(true);
const isHovered = ref(false);

// Reactive state for copy feedback
const copyStatus = ref("");
const isCopying = ref(false); // To prevent rapid clicks

const customStyle = ref({});

// Computed property for resize handle visibility
const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
}));

// Font size control
const currentFontSize = ref(12); // Default font size
const minFontSize = 10;
const maxFontSize = 24;
const fontSizeStep = 2;

const increaseFontSize = () => {
    currentFontSize.value = Math.min(currentFontSize.value + fontSizeStep, maxFontSize);
};

const decreaseFontSize = () => {
    currentFontSize.value = Math.max(currentFontSize.value - fontSizeStep, minFontSize);
};

// Configure marked to handle line breaks properly
marked.setOptions({
    breaks: false, // Disable GitHub Flavored Line Breaks
    gfm: true, // Enable GitHub Flavored Markdown
    headerIds: false, // Disable automatic header IDs if not needed
});

// Computed property to convert markdown to HTML
const markdownToHtml = computed(() => {
    return marked(response.value);
});

// ----- Computed properties -----
const response = computed({
    get: () => props.data.inputs.response,
    set: (value) => {
        props.data.inputs.response = value
    },
})

// Emit updates to parent components
const emitUpdate = () => {
    emit("update:data", { id: props.id, data: { ...props.data } });
};

// Function to scroll to the bottom of the text container
const scrollToBottom = () => {
    nextTick(() => {
        if (textContainer.value) {
            textContainer.value.scrollTop = textContainer.value.scrollHeight;
        }
    });
};

// Handle scroll events to toggle auto-scroll
const handleScroll = () => {
    if (textContainer.value) {
        const { scrollTop, scrollHeight, clientHeight } = textContainer.value;
        if (scrollTop + clientHeight < scrollHeight) {
            isAutoScrollEnabled.value = false;
        } else {
            isAutoScrollEnabled.value = true;
        }
    }
};

// Access zoom functions from VueFlow
const { zoomIn, zoomOut } = useVueFlow();

// Disable zoom when interacting with the text container
const disableZoom = () => {
    zoomIn(0);
    zoomOut(0);
};

// Enable zoom when not interacting
const enableZoom = () => {
    zoomIn(1);
    zoomOut(1);
};

// Handle resize events
const onResize = (event) => {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
};

// Method to copy text to clipboard
const copyToClipboard = async () => {
    if (isCopying.value) return; // Prevent multiple clicks
    isCopying.value = true;

    try {
        await navigator.clipboard.writeText(response.value);
        copyStatus.value = "Copied!";
    } catch (error) {
        console.error("Failed to copy text: ", error);
        copyStatus.value = "Failed to copy.";
    }

    // Clear the status message after 2 seconds
    setTimeout(() => {
        copyStatus.value = "";
        isCopying.value = false;
    }, 2000);
};

// Watch for changes and emit them upward
watch(
    () => props.data,
    (newData) => {
        emit('update:data', { id: props.id, data: newData })
        if (isAutoScrollEnabled.value) {
            scrollToBottom();
        }
    },
    { deep: true }
)
</script>

<style scoped>
.gemini-response-node {
    background-color: #333;
    border: 1px solid #666;
    border-radius: 4px;
    color: #eee;
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
}

.header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px;
    position: relative;
}

h3 {
    font-size: 14px;
    margin: 0;
}

.controls {
    display: flex;
    align-items: center;
}

.select-container {
    display: flex;
    align-items: center;
    margin-left: auto;
    margin-right: 10px;
}

label {
    font-size: 12px;
    margin-right: 5px;
}

select {
    background-color: #222;
    border: 1px solid #666;
    color: #eee;
    font-size: 12px;
    padding: 2px 5px;
}

/* Font Size Controls */
.font-size-controls {
    display: flex;
    gap: 5px;
    margin-right: 10px;
}

.font-size-controls button {
    background-color: #444;
    border: 1px solid #666;
    color: #eee;
    font-size: 12px;
    padding: 5px 8px;
    border-radius: 3px;
    cursor: pointer;
    transition: background-color 0.3s;
}

.font-size-controls button:hover {
    background-color: #555;
}

/* Styling for the Copy Button */
.copy-button {
    background-color: #444;
    border: 1px solid #666;
    color: #eee;
    font-size: 12px;
    padding: 5px 10px;
    border-radius: 3px;
    cursor: pointer;
    transition: background-color 0.3s;
}

.copy-button:hover {
    background-color: #555;
}

.copy-button:disabled {
    background-color: #555;
    cursor: not-allowed;
}

/* Styling for the Copy Feedback Message */
.copy-feedback {
    position: absolute;
    top: 40px;
    right: 10px;
    background-color: #555;
    color: #fff;
    padding: 3px 8px;
    border-radius: 3px;
    font-size: 10px;
    opacity: 0.9;
}

.node-container {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
}

.text-container {
    flex-grow: 1;
    overflow-y: auto;
    padding: 10px;
    margin-top: 0;
    margin-bottom: 0;
    width: auto;
    height: auto;
    min-height: 0;
    max-height: none;
    white-space: normal;
    text-align: left;
    /* Font size will be applied via inline style */
}

.raw-text,
.markdown-text {
    line-height: 1.5;
    /* Font size is inherited from the parent .text-container */
}

/* Optional: Add styles to ensure markdown renders correctly */
.markdown-text img {
    max-width: 100%;
    height: auto;
}

.markdown-text a {
    color: #1e90ff;
    text-decoration: underline;
}

.markdown-text h1,
.markdown-text h2,
.markdown-text h3,
.markdown-text h4,
.markdown-text h5,
.markdown-text h6 {
    color: #fff;
}

.markdown-text ul,
.markdown-text ol {
    padding-left: 20px;
}

.markdown-text blockquote {
    border-left: 4px solid #555;
    padding-left: 10px;
    color: #ccc;
}
</style>