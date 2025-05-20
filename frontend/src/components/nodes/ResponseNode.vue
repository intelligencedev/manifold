<template>
  <div
    :style="computedContainerStyle"
    class="node-container response-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-purple-400 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ modelTypeLabel }}</div>

    <div class="header flex flex-col gap-2 mb-2">
      <div class="controls flex flex-wrap gap-2 items-center">
        <div class="select-container flex flex-col">
          <label for="model-type" class="text-xs mb-1">Model:</label>
          <select id="model-type" v-model="selectedModelType" class="bg-zinc-800 border border-gray-600 rounded px-2 py-1 text-sm">
            <option value="openai">OpenAI</option>
            <option value="claude">Claude</option>
            <option value="gemini">Gemini</option>
          </select>
        </div>
        <div class="select-container flex flex-col">
          <label for="render-mode" class="text-xs mb-1">Render Mode:</label>
          <select id="render-mode" v-model="selectedRenderMode" class="bg-zinc-800 border border-gray-600 rounded px-2 py-1 text-sm">
            <option value="markdown">Markdown</option>
            <option value="raw">Raw Text</option>
            <option value="html">HTML</option>
          </select>
        </div>
        <div class="select-container flex flex-col" v-if="selectedRenderMode === 'markdown'">
          <label for="code-theme" class="text-xs mb-1">Theme:</label>
          <select id="code-theme" v-model="selectedTheme" class="bg-zinc-800 border border-gray-600 rounded px-2 py-1 text-sm">
            <option value="atom-one-dark">Dark</option>
            <option value="atom-one-light">Light</option>
            <option value="github">GitHub</option>
            <option value="monokai">Monokai</option>
            <option value="vs">VS</option>
          </select>
        </div>
        <div class="font-size-controls flex gap-1 items-center">
          <button @click.prevent="decreaseFontSize" class="px-2 py-1 rounded bg-purple-700 hover:bg-purple-800 text-xs">-</button>
          <button @click.prevent="increaseFontSize" class="px-2 py-1 rounded bg-purple-700 hover:bg-purple-800 text-xs">+</button>
        </div>
        <button class="copy-button px-3 py-1 rounded bg-purple-600 hover:bg-purple-700 text-white text-xs" @click="copyToClipboard" :disabled="isCopying">
          Copy
        </button>
      </div>
      <div v-if="copyStatus" class="copy-feedback text-xs text-green-400">{{ copyStatus }}</div>
    </div>

    <div
      class="flex-1 text-container overflow-auto rounded bg-zinc-800 p-2 mb-2"
      ref="textContainer"
      @scroll="handleScroll"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
      :style="{ fontSize: `${currentFontSize}px` }"
    >
      <!-- Rendered content (markdown, raw, html, etc.) -->
      <slot />
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <NodeResizer
      :is-resizable="true"
      :color="'#a78bfa'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="320"
      :min-height="180"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { reactive, watch, ref, computed, nextTick, onMounted } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { marked } from "marked";
import { NodeResizer } from "@vue-flow/node-resizer";
import hljs from 'highlight.js';
import DOMPurify from 'dompurify';

const { getEdges, findNode, updateNodeData } = useVueFlow()

// Theme selection
const selectedTheme = ref('atom-one-dark');
const selectedModelType = ref('openai');
let currentThemeLink = null;

/**
 * @typedef {Object} ThinkingBlock
 * @property {string} content - Full untrimmed content
 * @property {string} preview - Last two lines of content
 * @property {boolean} hasMore - Whether there are more than two lines
 * @property {boolean} collapsed - Current collapsed state
 */

// Model type label computed property
const modelTypeLabel = computed(() => {
    const labels = {
        openai: 'OpenAI Response',
        claude: 'Claude Response',
        gemini: 'Gemini Response'
    };
    return labels[selectedModelType.value] || 'Response';
});

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
        props.data.run = run;
    }
    
    // Load initial theme
    loadTheme(selectedTheme.value);
    
    // Configure marked to use highlight.js for code highlighting
    marked.setOptions({
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
});

// Watch for theme changes
watch(selectedTheme, (newTheme) => {
    loadTheme(newTheme);
});

// Watch for model type changes
watch(selectedModelType, (newType) => {
    props.data.type = `${newType}Response`;
    updateNodeData();
});

async function run() {
    console.log("Running ResponseNode:", props.id);

    // Get target edges
    const targetEdges = getEdges.value;

    // Get the source node
    const connectedSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => edge.source);

    if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0]);

        console.log("Source node:", sourceNode);

        if (sourceNode.data.outputs.result) {
            // Get the response from the source node
            props.data.inputs.response = sourceNode.data.outputs.result.output;

            // The AgentNode looks for sourceNode.data.outputs.result.output
            // So, store your aggregated text (or whatever you want) in the same structure:
            props.data.outputs = {
                result: {
                    output: response.value, // or results, or both, depending on your preference
                },
            }

            // Increment reRenderKey here.
            reRenderKey.value++;

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

// Function to process and extract text content from Claude's streaming responses
function processClaudeStreamingResponse(input) {
  // Early return if the input doesn't match expected format
  if (!input.includes('event:')) {
    return input;
  }

  // Extracted text content
  let extractedText = '';
  
  // Split the input by lines to process each event
  const lines = input.split('\n');
  let i = 0;
  
  while (i < lines.length) {
    const line = lines[i].trim();
    
    // Process content_block_delta events to extract text
    if (line.startsWith('event: content_block_delta')) {
      // The next line should contain the data
      if (i + 1 < lines.length) {
        const dataLine = lines[i + 1];
        if (dataLine.startsWith('data:')) {
          try {
            // Extract the JSON data
            const jsonStr = dataLine.substring(dataLine.indexOf('{'));
            const data = JSON.parse(jsonStr);
            
            // Check if it's a text delta and extract the text
            if (data.type === 'content_block_delta' && 
                data.delta && 
                data.delta.type === 'text_delta' &&
                data.delta.text) {
              extractedText += data.delta.text;
            }
          } catch (e) {
            console.error('Error parsing Claude SSE JSON:', e);
          }
        }
      }
    }
    
    i++;
  }
  
  return extractedText;
}

// Define props and emits
const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'Response_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'ResponseNode',
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
                width: '624px',
                height: '400px',
            },
        }),
    },
})
const emit = defineEmits(["update:data", "disable-zoom", "enable-zoom", "resize"]);

// Reactive state for render mode
const selectedRenderMode = ref("markdown");

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

// Configure marked to handle line breaks properly
marked.setOptions({
    breaks: false, // Disable GitHub Flavored Line Breaks
    gfm: true, // Enable GitHub Flavored Markdown
    headerIds: false, // Disable automatic header IDs if not needed
});

// ADDED: Reactive key to force re-render
const reRenderKey = ref(0);

// ---------------- reactive state --------------
const thinkingBlocks = ref([]);

// text *outside* any <think> tag
const outsideThinkingRaw = ref('');

// ---------------- parser ----------------------
function parseResponse(txt) {
    // Store previous collapsed states to maintain them during updates
    const previousStates = thinkingBlocks.value.map(block => block.collapsed);
    
    // Special handling for Claude messages if selectedModelType is claude
    if (selectedModelType.value === 'claude' && txt.includes('event:')) {
        const processedText = processClaudeStreamingResponse(txt);
        
        // Now continue with normal parsing using the processed text
        const blocks = [];
        const outside = [];
        const regex = /<(?:think|thinking)>([\s\S]*?)(?:<\/(?:think|thinking)>|$)/gi;
        let lastIndex = 0;
        let match;
        let blockIndex = 0;
        
        while ((match = regex.exec(processedText)) !== null) {
            // text before this block
            if (match.index > lastIndex) {
                outside.push(processedText.slice(lastIndex, match.index));
            }
            
            const full = match[1].trimEnd();
            const lines = full.split('\n');
            const preview = lines.slice(-2).join('\n');
            
            // Use previous collapsed state if available, otherwise default to collapsed
            const wasCollapsed = blockIndex < previousStates.length ? 
                                previousStates[blockIndex] : 
                                true;
            
            blocks.push({ 
                content: full, 
                preview, 
                hasMore: lines.length > 2, 
                collapsed: wasCollapsed  // Preserve previous state
            });
            
            lastIndex = match.index + match[0].length;
            blockIndex++;
        }
        
        // trailing text
        if (lastIndex < processedText.length) {
            outside.push(processedText.slice(lastIndex));
        }
        
        thinkingBlocks.value = blocks;
        outsideThinkingRaw.value = outside.join('');
        return;
    }
    
    const blocks = [];
    const outside = [];
    const regex = /<(?:think|thinking)>([\s\S]*?)(?:<\/(?:think|thinking)>|$)/gi;
    let lastIndex = 0;
    let match;
    let blockIndex = 0;
    
    while ((match = regex.exec(txt)) !== null) {
        // text before this block
        if (match.index > lastIndex) {
            outside.push(txt.slice(lastIndex, match.index));
        }
        
        const full = match[1].trimEnd();
        const lines = full.split('\n');
        const preview = lines.slice(-2).join('\n');
        
        // Use previous collapsed state if available, otherwise default to collapsed
        const wasCollapsed = blockIndex < previousStates.length ? 
                            previousStates[blockIndex] : 
                            true;
        
        blocks.push({ 
            content: full, 
            preview, 
            hasMore: lines.length > 2, 
            collapsed: wasCollapsed  // Preserve previous state
        });
        
        lastIndex = match.index + match[0].length;
        blockIndex++;
    }
    
    // trailing text
    if (lastIndex < txt.length) {
        outside.push(txt.slice(lastIndex));
    }
    
    thinkingBlocks.value = blocks;
    outsideThinkingRaw.value = outside.join('');
}

// ---------------- derived HTML ----------------
const markdownOutsideThinking = computed(() =>
    marked(outsideThinkingRaw.value)
);

const htmlOutsideThinking = computed(() =>
    DOMPurify.sanitize(outsideThinkingRaw.value)
);

// ---------------- toggle handler --------------
function toggleThink(idx) {
    thinkingBlocks.value[idx].collapsed = !thinkingBlocks.value[idx].collapsed;
}

// Define the response computed property
const response = computed(() => props.data.inputs.response || '');

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

// Watch for render mode changes
watch(selectedRenderMode, () => {
    // Re-render if needed
    nextTick(() => {
        if (isAutoScrollEnabled.value) {
            scrollToBottom();
        }
    });
});

// ---------------- watch the stream ------------
watch(
    () => props.data.inputs.response,
    (newResponseText, oldResponseText) => {
        parseResponse(newResponseText || '');
        nextTick(() => {
            if (isAutoScrollEnabled.value) {
                scrollToBottom();
            }
        });
    },
    { immediate: true }
);

watch(response, () => {
  nextTick(() => {
    document.querySelectorAll('pre code:not(.hljs)').forEach((block) => { hljs.highlightElement(block); });
  });
});

</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>