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

                <!-- Theme Selector -->
                <div class="select-container" v-if="selectedRenderMode === 'markdown'">
                    <label for="code-theme">Theme:</label>
                    <select id="code-theme" v-model="selectedTheme">
                        <option value="atom-one-dark">Dark</option>
                        <option value="atom-one-light">Light</option>
                        <option value="github">GitHub</option>
                        <option value="monokai">Monokai</option>
                        <option value="vs">VS</option>
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
            @mouseleave="$emit('enable-zoom')" @wheel.stop :style="{ fontSize: `${currentFontSize}px` }">
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
import hljs from 'highlight.js';

const { getEdges, findNode, updateNodeData } = useVueFlow()

// Theme selection
const selectedTheme = ref('atom-one-dark');
let currentThemeLink = null;

// Load highlight.js theme
const loadTheme = (themeName) => {
    // Remove the previous theme if it exists
    if (currentThemeLink) {
        document.head.removeChild(currentThemeLink);
    }
    
    // Create and append the new theme link
    currentThemeLink = document.createElement('link');
    currentThemeLink.rel = 'stylesheet';
    currentThemeLink.href = `https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.7.0/styles/${themeName}.min.css`;
    document.head.appendChild(currentThemeLink);
};

onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
    
    // Load initial theme
    loadTheme(selectedTheme.value);
    
    // Configure marked to use highlight.js for code highlighting
    marked.setOptions({
        breaks: true, 
        gfm: true,
        headerIds: false,
        highlight: function(code, lang) {
            if (lang && hljs.getLanguage(lang)) {
                try {
                    return hljs.highlight(code, { language: lang }).value;
                } catch (e) {
                    console.error(e);
                }
            }
            // Use auto-detection if language isn't specified
            try {
                return hljs.highlightAuto(code).value;
            } catch (e) {
                console.error(e);
            }
            return code; // Return original if highlighting fails
        }
    });
})

// Watch for theme changes
watch(selectedTheme, (newTheme) => {
    loadTheme(newTheme);
});

async function run() {
    console.log("Running GeminiResponse:", props.id);

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

            // Make the response available downstream
            props.data.outputs = {
                result: {
                    output: response.value
                },
            }
            
            // Increment reRenderKey to force re-render
            reRenderKey.value++;
            
            updateNodeData();
        }
    }

    // Make the response available downstream
    props.data.outputs = {
        result: {
            output: response.value
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
                borderRadius: '12px',
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
const selectedRenderMode = ref("markdown"); // Changed default to markdown

// References to DOM elements
const textContainer = ref(null);

// Auto-scroll control
const isAutoScrollEnabled = ref(true);
const isHovered = ref(false);

// Reactive state for copy feedback
const copyStatus = ref("");
const isCopying = ref(false); // To prevent rapid clicks

const customStyle = ref({});

// Reactive key to force re-render
const reRenderKey = ref(0);

// Computed property for resize handle visibility
const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
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

// Computed property to convert markdown to HTML
const markdownToHtml = computed(() => {
    // Access reRenderKey to force re-evaluation
    reRenderKey.value;
    return marked(response.value);
});

// ----- Computed properties -----
const response = computed({
    get: () => props.data.inputs.response,
    set: (value) => {
        props.data.inputs.response = value
    },
})

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
        isAutoScrollEnabled.value = scrollTop + clientHeight >= scrollHeight - 10;
    }
};

// Access zoom functions from VueFlow
const { zoomIn, zoomOut } = useVueFlow();

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
);

// Watch for render mode changes
watch(selectedRenderMode, () => {
    // Re-render if needed
    nextTick(() => {
        if (isAutoScrollEnabled.value) {
            scrollToBottom();
        }
    });
});

// Apply syntax highlighting after response updates
watch(response, () => {
    nextTick(() => {
        hljs.highlightAll();
    });
});
</script>

<style>
/* Note: removed 'scoped' to allow proper application of syntax highlighting */
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

.node-label {
    color: var(--node-text-color);
    font-size: 16px;
    text-align: center;
    margin-bottom: 10px;
    font-weight: bold;
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
}

.raw-text,
.markdown-text {
    line-height: 1.5;
}

/* Ensure markdown renders correctly */
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
    margin-top: 16px;
    margin-bottom: 8px;
}

.markdown-text ul,
.markdown-text ol {
    padding-left: 20px;
    margin-bottom: 16px;
}

.markdown-text blockquote {
    border-left: 4px solid #555;
    padding-left: 10px;
    margin-left: 0;
    color: #ccc;
}

.markdown-text code {
    background-color: #444;
    padding: 2px 4px;
    border-radius: 3px;
    font-family: monospace;
}

.markdown-text pre {
    background-color: #222;
    padding: 10px;
    border-radius: 4px;
    overflow-x: auto;
}

.markdown-text pre code {
    background-color: transparent;
    padding: 0;
}

.markdown-text table {
    border-collapse: collapse;
    width: 100%;
    margin-bottom: 16px;
}

.markdown-text th,
.markdown-text td {
    border: 1px solid #555;
    padding: 8px;
    text-align: left;
}

.markdown-text th {
    background-color: #444;
}

/* Syntax highlighting styles */
.hljs {
    padding: 12px;
    border-radius: 5px;
    overflow-x: auto;
}

pre {
    margin: 10px 0;
    border-radius: 5px;
    overflow-x: auto;
}

code {
    font-family: monospace;
}
</style>