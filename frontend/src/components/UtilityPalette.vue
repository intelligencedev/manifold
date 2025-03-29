<template>
  <div class="utility-palette" :class="{ 'is-open': isEditorOpen }">
    <div class="toggle-button" @click="togglePalette">
      <svg v-if="isEditorOpen" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
        viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
        viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708z" />
      </svg>
    </div>
    <div class="utility-content">
      <div class="wasm-code-editor-container">
        <!-- Toolbar -->
        <div class="toolbar">
          <div class="language-selector">
            <label for="lang-select">Language:</label>
            <select id="lang-select" v-model="selectedLanguage" @change="handleLanguageChange">
              <option value="javascript">JavaScript</option>
              <option value="html">HTML</option>
            </select>
          </div>
          <div class="action-buttons">
            <button @click="runCode" :disabled="isRunning || !quickJSVm">
              {{ selectedLanguage === 'html' ? 'Render' : 'Run' }}
            </button>
            <button @click="clearOutput">Clear Output</button>
          </div>
        </div>
    
        <!-- CodeMirror Editor -->
        <div class="editor-wrapper" :style="{ height: editorHeightPercent + '%' }">
          <Codemirror
            v-model="code"
            placeholder="Enter JavaScript code..."
            :style="{ height: '100%' }"
            :autofocus="true"
            :indent-with-tab="true"
            :tab-size="2"
            :extensions="cmExtensions"
            @ready="handleCmReady"
          />
        </div>
        
        <!-- Resizable divider -->
        <div 
          class="resize-handle" 
          @mousedown="startResize"
          @touchstart="startResize"
        >
          <div class="handle-indicator"></div>
        </div>
    
        <!-- Output Area (Console or HTML Preview) -->
        <div class="output-area" :style="{ height: (100 - editorHeightPercent) + '%' }">
          <div class="output-header">
            <h3>{{ selectedLanguage === 'html' ? 'HTML Preview' : 'Output / Logs' }}</h3>
            <div v-if="selectedLanguage === 'html'" class="preview-controls">
              <button @click="refreshPreview" title="Refresh Preview">↻</button>
            </div>
          </div>
          <pre v-if="selectedLanguage !== 'html'" ref="outputRef" class="output-content">{{ output }}</pre>
          <div v-else class="html-preview-container">
            <iframe ref="htmlPreviewRef" class="html-preview-iframe" sandbox="allow-scripts"></iframe>
          </div>
        </div>
    
        <!-- Status/Loading -->
        <div v-if="isLoadingWasm" class="status-message">Loading Wasm Engine...</div>
        <div v-if="wasmError" class="error-message">Wasm Error: {{ wasmError }}</div>
        <div v-if="isRunning" class="status-message">Running...</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, onMounted, computed, nextTick, onUnmounted, watch } from 'vue';
import { Codemirror } from 'vue-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { html } from '@codemirror/lang-html';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { getQuickJS, QuickJSContext, QuickJSWASMModule } from 'quickjs-emscripten';
import { useCodeEditor } from '@/composables/useCodeEditor';

// Use the global code editor composable
const { code, isEditorOpen, openEditor, closeEditor } = useCodeEditor();

// --- Refs ---
const output = ref<string>('');
const isRunning = ref<boolean>(false);
const isLoadingWasm = ref<boolean>(true);
const wasmError = ref<string | null>(null);
const quickJSVm = shallowRef<QuickJSContext | null>(null); // Use shallowRef for complex non-reactive objects
const outputRef = ref<HTMLPreElement | null>(null);
const htmlPreviewRef = ref<HTMLIFrameElement | null>(null);
const cmView = shallowRef<EditorView>(); // To access CodeMirror view instance if needed
const selectedLanguage = ref<'javascript' | 'html'>('javascript');
// Heights for resizable panels
const editorHeightPercent = ref<number>(50); // Default split at 50%
const htmlCode = ref<string>(`<!DOCTYPE html>
<html>
<head>
  <style>
    body {
      font-family: Arial, sans-serif;
      padding: 20px;
      line-height: 1.6;
    }
    h1 {
      color: #336699;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      border: 1px solid #ccc;
      padding: 20px;
      border-radius: 5px;
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>HTML Preview</h1>
    <p>This is a live preview of your HTML code. Edit the code and click 'Render' to see changes.</p>
    <ul>
      <li>Add your own HTML elements</li>
      <li>Style with CSS in the head section</li>
      <li>See changes in real-time</li>
    </ul>
  </div>
</body>
</html>`);

// Track code for different languages
const codeStore = {
  javascript: "console.log('Hello from Manifold!');",
  html: htmlCode.value
};

// --- CodeMirror ---
const cmTheme = oneDark; // Make this selectable later if needed
const cmExtensions = computed(() => {
  const langSupport = selectedLanguage.value === 'html' ? html() : javascript();
  return [
    langSupport,
    cmTheme,
    EditorView.lineWrapping, // Enable line wrapping
    // Add other extensions like line numbers, keymaps, etc.
    EditorView.theme({ // Basic styling adjustments
      '&': {
        fontSize: '13px',
        height: '100%', // Ensure it fills wrapper
      },
      '.cm-scroller': { overflow: 'auto' },
      '.cm-content': { 
        textAlign: 'left', // Ensure text is left-aligned
        justifyContent: 'flex-start' // Align lines to the left
      },
      '.cm-line': {
        textAlign: 'left', // Ensure each line is left-aligned
        justifyContent: 'flex-start' // Left-justify content within lines
      }
    }),
  ];
});

const handleCmReady = (payload: any) => {
  cmView.value = payload.view;
  console.log('CodeMirror instance ready');
};

// --- Wasm Engine ---
onMounted(async () => {
  try {
    isLoadingWasm.value = true;
    wasmError.value = null;
    const QuickJS = await getQuickJS();
    quickJSVm.value = QuickJS.newContext();
    console.log('QuickJS Wasm engine initialized.');
  } catch (error: any) {
    console.error("Failed to load QuickJS Wasm:", error);
    wasmError.value = error.message || 'Unknown error loading Wasm.';
  } finally {
    isLoadingWasm.value = false;
  }
});

// Cleanup Wasm VM on component unmount
onUnmounted(() => {
  if (quickJSVm.value) {
    quickJSVm.value.dispose();
    quickJSVm.value = null;
    console.log('QuickJS Wasm VM disposed.');
  }
});

// --- Core Logic ---
function togglePalette() {
  isEditorOpen.value = !isEditorOpen.value;
}

// Handle language switching
const handleLanguageChange = () => {
  // Save current code for the old language
  if (selectedLanguage.value === 'javascript') {
    codeStore.html = code.value;
    code.value = codeStore.javascript;
  } else {
    codeStore.javascript = code.value;
    code.value = codeStore.html;
    
    // When switching to HTML, render the preview
    nextTick(() => {
      renderHtml();
    });
  }
};

// Watch for language changes to update the placeholder and editor
watch(selectedLanguage, (newLang) => {
  // When switching to HTML, render it automatically
  if (newLang === 'html') {
    nextTick(renderHtml);
  }
});

// HTML rendering function
const renderHtml = () => {
  const iframe = htmlPreviewRef.value as HTMLIFrameElement;
  if (!iframe) return;
  
  try {
    // Instead of accessing iframe document directly, use srcdoc attribute
    // which avoids cross-origin issues
    iframe.srcdoc = code.value;
  } catch (error: any) {
    console.error('Error rendering HTML:', error);
    
    // Show error in iframe
    iframe.srcdoc = `
      <html>
      <head>
        <style>
          body {
            font-family: Arial, sans-serif;
            background-color: #f8f8f8;
            color: #333;
            padding: 20px;
          }
          .error-container {
            border: 2px solid #ff6b6b;
            border-radius: 4px;
            padding: 20px;
            background-color: rgba(255, 107, 107, 0.1);
          }
          h3 {
            color: #ff6b6b;
            margin-top: 0;
          }
        </style>
      </head>
      <body>
        <div class="error-container">
          <h3>Error Rendering HTML</h3>
          <p>${error.message || 'An unknown error occurred'}</p>
        </div>
      </body>
      </html>
    `;
  }
};

// Function to manually refresh the preview
const refreshPreview = () => {
  if (selectedLanguage.value === 'html') {
    renderHtml();
  }
};

const runCode = () => {
  // If HTML mode, render the HTML instead of running JavaScript
  if (selectedLanguage.value === 'html') {
    renderHtml();
    return;
  }
  
  // JavaScript execution code
  if (!quickJSVm.value || isRunning.value) return;

  isRunning.value = true;
  output.value = `Executing at ${new Date().toLocaleTimeString()}...\n\n`;
  scrollToOutputBottom();

  // Use setTimeout to allow UI update before potentially blocking execution
  setTimeout(() => {
    try {
      const vm = quickJSVm.value as QuickJSContext; // Type assertion

      // Capture console.log, console.error, etc.
      // We need to expose functions from JS host (Vue) to Wasm guest (QuickJS)
      const logHandler = vm.newFunction("log", (...args) => {
        const formattedArgs = args.map(arg => vm.dump(arg));
        output.value += `[LOG] ${formattedArgs.join(' ')}\n`;
        scrollToOutputBottom();
      });
      const errorHandler = vm.newFunction("error", (...args) => {
        const formattedArgs = args.map(arg => vm.dump(arg));
        output.value += `[ERR] ${formattedArgs.join(' ')}\n`;
        scrollToOutputBottom();
      });

      // Expose the handlers to the global scope inside QuickJS
      const consoleObj = vm.newObject();
      vm.setProp(consoleObj, "log", logHandler);
      vm.setProp(consoleObj, "error", errorHandler);
      vm.setProp(consoleObj, "warn", logHandler); // Map warn to log for simplicity
      vm.setProp(vm.global, "console", consoleObj);

      // Release handles for the functions and object
      logHandler.dispose();
      errorHandler.dispose();
      consoleObj.dispose();

      // Execute the code
      const result = vm.evalCode(code.value);

      if (result.error) {
        output.value += `\n--- EXECUTION ERROR ---\n`;
        const errorVal = vm.dump(result.error);
        output.value += `${errorVal.name}: ${errorVal.message}\n`;
        if (errorVal.stack) {
            output.value += `Stack:\n${errorVal.stack}\n`;
        }
        result.error.dispose();
      } else {
        output.value += `\n--- EXECUTION SUCCESS ---\n`;
        const resultVal = vm.dump(result.value);
        output.value += `Return Value: ${JSON.stringify(resultVal, null, 2)}\n`;
        result.value.dispose();
      }

    } catch (error: any) {
      output.value += `\n--- HOST ERROR ---\n`;
      output.value += error.message || 'An unexpected error occurred.';
      console.error("Error running Wasm code:", error);
    } finally {
      isRunning.value = false;
      scrollToOutputBottom();
    }
  }, 10); // Small delay
};

const clearOutput = () => {
  output.value = '';
};

const scrollToOutputBottom = () => {
    nextTick(() => {
        if (outputRef.value) {
            outputRef.value.scrollTop = outputRef.value.scrollHeight;
        }
    });
};

// --- Resizing Functionality ---
let isResizing = false;
let lastClientY = 0;

// Start resize operation
const startResize = (event: MouseEvent | TouchEvent) => {
  isResizing = true;
  
  // Store the initial mouse/touch position
  lastClientY = event instanceof MouseEvent ? event.clientY : event.touches[0].clientY;
  
  // Add event listeners
  document.addEventListener('mousemove', handleResize);
  document.addEventListener('touchmove', handleResize, { passive: false });
  document.addEventListener('mouseup', stopResize);
  document.addEventListener('touchend', stopResize);
  
  // Prevent default to avoid text selection
  event.preventDefault();
};

// Handle resize movement
const handleResize = (event: MouseEvent | TouchEvent) => {
  if (!isResizing) return;
  
  // Get current mouse/touch position
  const clientY = event instanceof MouseEvent ? event.clientY : event.touches[0].clientY;
  
  // Get container information for calculation
  const container = document.querySelector('.wasm-code-editor-container');
  if (!container) return;
  
  const containerRect = container.getBoundingClientRect();
  const containerHeight = containerRect.height;
  const minHeight = 50; // Minimum height in pixels for each section
  
  // Calculate how much the mouse has moved
  const deltaY = clientY - lastClientY;
  
  // Calculate the percentage change based on container height
  const deltaPercent = (deltaY / containerHeight) * 100;
  
  // Calculate new percentage, applying constraints
  const newPercent = Math.min(
    Math.max(
      (minHeight / containerHeight) * 100,
      editorHeightPercent.value + deltaPercent
    ),
    100 - (minHeight / containerHeight) * 100
  );
  
  // Update percentage
  editorHeightPercent.value = newPercent;
  
  // Update last position
  lastClientY = clientY;
  
  // Prevent scrolling on touch devices
  if (event instanceof TouchEvent) {
    event.preventDefault();
  }
};

// Stop resize operation
const stopResize = () => {
  isResizing = false;
  document.removeEventListener('mousemove', handleResize);
  document.removeEventListener('touchmove', handleResize);
  document.removeEventListener('mouseup', stopResize);
  document.removeEventListener('touchend', stopResize);
};
</script>

<style scoped>
/* Utility Palette Container */
.utility-palette {
  position: fixed;
  top: 50px;
  bottom: 0;
  right: 0;
  width: 50%; /* Changed from 250px to 50% of viewport */
  background-color: #222;
  color: #eee;
  z-index: 1100;
  transition: transform 0.3s ease-in-out;
  transform: translateX(100%);
  font-family: 'Roboto', sans-serif; /* Consistent font */
  box-shadow: -5px 0 15px rgba(0, 0, 0, 0.3); /* Add shadow for better visuals */
}

.utility-palette.is-open {
  transform: translateX(0);
}

/* Toggle Button */
.toggle-button {
  position: absolute;
  top: 50%;
  left: -30px;
  width: 30px;
  height: 60px;
  background-color: #222;
  border-right: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-top-left-radius: 8px;
  border-bottom-left-radius: 8px;
  box-shadow: -2px 0 5px rgba(0, 0, 0, 0.2); /* Add shadow */
}

.toggle-button svg {
  fill: #eee;
  width: 16px;
  height: 16px;
}

/* Utility Content */
.utility-content {
  height: 100%;
  box-sizing: border-box;
  overflow: hidden; /* Prevent content overflow */
}

/* Code Editor Styles */
.wasm-code-editor-container {
  display: flex;
  flex-direction: column;
  height: 100%; /* Fill parent (UtilityPalette) */
  background-color: #2a2a2a; /* Match palette */
  color: #eee;
  font-family: sans-serif;
  padding: 10px;
  box-sizing: border-box;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #444;
}

.language-selector {
  display: flex;
  align-items: center;
  gap: 8px;
}

.language-selector label {
  font-size: 0.9em;
}

.language-selector select {
  padding: 4px 8px;
  background-color: #333;
  color: #eee;
  border: 1px solid #555;
  border-radius: 4px;
  font-size: 0.9em;
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.editor-wrapper {
  flex-shrink: 1; /* Allow shrinking */
  flex-grow: 1; /* Allow growing but less than output */
  min-height: 150px; /* Minimum editor height */
  height: 50%; /* Default height percentage */
  border: 1px solid #444;
  margin-bottom: 10px;
  overflow: hidden; /* Prevent editor from overflowing wrapper */
}

/* Style codemirror component */
:deep(.cm-editor) {
  height: 100%;
}

.output-area {
  flex-shrink: 1;
  flex-grow: 1; /* Take up remaining space */
  min-height: 100px;
  height: 45%; /* Default height percentage */
  display: flex;
  flex-direction: column;
  border: 1px solid #444;
  background-color: #1e1e1e; /* Darker bg for output */
  border-radius: 4px;
}

.output-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background-color: #333;
  border-bottom: 1px solid #444;
  border-top-left-radius: 4px;
  border-top-right-radius: 4px;
  padding: 0 10px;
}

.preview-controls {
  display: flex;
  gap: 4px;
}

.preview-controls button {
  background-color: transparent;
  border: none;
  color: #ccc;
  font-size: 16px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
}

.preview-controls button:hover {
  background-color: #444;
  color: #fff;
}

.html-preview-container {
  flex-grow: 1;
  height: 100%;
  position: relative;
  overflow: hidden;
  background-color: #fff;
}

.html-preview-iframe {
  width: 100%;
  height: 100%;
  border: none;
  background-color: white;
  box-sizing: border-box;
}
.status-message {
  padding: 5px;
  font-style: italic;
  color: #aaa;
  font-size: 0.9em;
}
.error-message {
  padding: 5px;
  font-weight: bold;
  color: #ff6b6b; /* Error color */
  font-size: 0.9em;
}

/* Resizable Divider */
.resize-handle {
  height: 6px;
  margin: 5px 0;
  cursor: ns-resize;
  background-color: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
}

.handle-indicator {
  width: 30px;
  height: 4px;
  border-radius: 2px;
  background-color: #555;
  margin: 0 auto;
}

.resize-handle:hover .handle-indicator {
  background-color: #777;
}

.output-content {
  flex-grow: 1;
  overflow: auto;
  padding: 10px;
  margin: 0;
  font-family: 'Courier New', monospace;
  font-size: 0.85em;
  white-space: pre-wrap;
  word-break: break-word;
  color: #ddd;
  background-color: #1a1a1a;
  text-align: left;
  line-height: 1.4;
  border-radius: 0 0 4px 4px;
  display: block;
  width: 100%;
  box-sizing: border-box;
}
</style>