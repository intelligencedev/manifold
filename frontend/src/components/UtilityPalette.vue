<template>
  <div class="utility-palette" :class="{ 'is-open': isOpen }">
    <div class="toggle-button" @click="togglePalette">
      <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
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
          <button @click="runCode" :disabled="isRunning || !quickJSVm">Run</button>
          <button @click="clearOutput">Clear Output</button>
        </div>
    
        <!-- CodeMirror Editor -->
        <div class="editor-wrapper">
          <Codemirror
            v-model="code"
            placeholder="Enter JavaScript code..."
            :style="{ height: '400px' }"
            :autofocus="true"
            :indent-with-tab="true"
            :tab-size="2"
            :extensions="cmExtensions"
            @ready="handleCmReady"
          />
        </div>
    
        <!-- Output Area -->
        <div class="output-area">
          <h3>Output / Logs</h3>
          <pre ref="outputRef" class="output-content">{{ output }}</pre>
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
import { ref, shallowRef, onMounted, computed, nextTick, onUnmounted } from 'vue';
import { Codemirror } from 'vue-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { getQuickJS, QuickJSContext, QuickJSWASMModule } from 'quickjs-emscripten';

// --- Refs ---
const isOpen = ref(false);
const code = ref<string>('console.log("Hello from Wasm!");\n// Try accessing window or document - it should fail\n// Example: console.log(window.location.href);\n');
const output = ref<string>('');
const isRunning = ref<boolean>(false);
const isLoadingWasm = ref<boolean>(true);
const wasmError = ref<string | null>(null);
const quickJSVm = shallowRef<QuickJSContext | null>(null); // Use shallowRef for complex non-reactive objects
const outputRef = ref<HTMLPreElement | null>(null);
const cmView = shallowRef<EditorView>(); // To access CodeMirror view instance if needed

// --- CodeMirror ---
const cmTheme = oneDark; // Make this selectable later if needed
const cmExtensions = computed(() => [
    javascript(),
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
]);

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
  isOpen.value = !isOpen.value;
}

const runCode = () => {
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
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #444;
}

.toolbar button {
  padding: 5px 10px;
  background-color: #444;
  color: #eee;
  border: 1px solid #666;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9em;
}

.toolbar button:hover:not(:disabled) {
  background-color: #555;
}

.toolbar button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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

.output-area h3 {
  margin: 0;
  padding: 8px 10px;
  font-size: 0.9em;
  background-color: #333;
  border-bottom: 1px solid #444;
  border-top-left-radius: 4px;
  border-top-right-radius: 4px;
}

.output-content {
  flex-grow: 1;
  overflow: auto;
  padding: 10px;
  margin: 0;
  font-family: 'Courier New', monospace;
  font-size: 0.85em;
  white-space: pre-wrap;
  word-break: break-all;
  color: #ccc;
  text-align: left; /* Ensure text is left-aligned */
}

/* Add specific styling for log entries to ensure they're left-aligned */
.output-content * {
  text-align: left;
}

.output-content::-webkit-scrollbar {
  width: 6px;
}
.output-content::-webkit-scrollbar-thumb {
  background-color: #555;
  border-radius: 3px;
}
.output-content::-webkit-scrollbar-track {
  background: #2a2a2a;
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
</style>