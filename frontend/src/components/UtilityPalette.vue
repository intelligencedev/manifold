<template>
  <div :class="['fixed top-[62px] bottom-0 w-1/2 right-0 z-[1100] transition-transform duration-300 text-white dark:bg-neutral-800 flex flex-col shadow-[-5px_0_10px_-5px_rgba(0,0,0,0.3)]', isEditorOpen ? 'translate-x-0' : 'translate-x-full']">
    <div class="absolute top-1/2 left-0 -translate-x-full w-[30px] h-[60px] dark:bg-neutral-800 rounded-l-md flex items-center justify-center cursor-pointer shadow-[-5px_0_10px_-5px_rgba(0,0,0,0.3)]" @click="togglePalette">
      <svg v-if="isEditorOpen" xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
      </svg>
    </div>
    <div class="p-4 h-full box-border flex flex-col">
      <div class="flex-1 flex flex-col gap-4">
        <!-- Toolbar -->
        <div class="flex justify-between items-center flex-wrap gap-2 mb-3 pb-3 border-b border-neutral-600">
          <div class="flex items-center gap-2">
            <label for="lang-select" class="text-sm">Language:</label>
            <select id="lang-select" v-model="selectedLanguage" @change="handleLanguageChange"
              class="p-1 bg-neutral-700 text-white border border-neutral-600 rounded text-sm">
              <option value="javascript">JavaScript</option>
              <option value="python">Python</option>
              <option value="html">HTML</option>
            </select>
          </div>
          <div class="flex gap-2">
            <button @click="runCode" :disabled="isRunning || !quickJSVm"
              class="px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-sm">
              {{ selectedLanguage === 'html' ? 'Render' : 'Run' }}
            </button>
            <button @click="clearOutput" class="px-3 py-1 bg-neutral-700 text-white rounded hover:bg-neutral-600 text-sm">
              Clear Output
            </button>
          </div>
        </div>

        <!-- CodeMirror Editor -->
        <div class="flex-1 min-h-[150px] border border-neutral-600 rounded mb-2 overflow-hidden"
          :style="{ height: editorHeightPercent + '%' }">
          <Codemirror v-model="code" placeholder="Enter JavaScript code..." class="h-full" :autofocus="true"
            :indent-with-tab="true" :tab-size="2" :extensions="cmExtensions" @ready="handleCmReady" />
        </div>

        <!-- Resizable divider -->
        <div class="h-2 bg-neutral-700 my-1 cursor-ns-resize flex items-center justify-center" @mousedown="startResize"
          @touchstart="startResize">
          <div class="w-10 h-1 bg-neutral-500 rounded-full"></div>
        </div>

        <!-- Output Area (Console or HTML Preview) -->
        <div class="flex-1 min-h-[100px] border border-neutral-600 rounded flex flex-col bg-neutral-900"
          :style="{ height: (100 - editorHeightPercent) + '%' }">
          <div class="flex justify-between items-center bg-neutral-700 border-b border-neutral-600 rounded-t px-3 py-2">
            <h3 class="text-sm font-medium">{{ selectedLanguage === 'html' ? 'HTML Preview' : 'Output / Logs' }}</h3>
            <div v-if="selectedLanguage === 'html'" class="flex gap-1">
              <button @click="refreshPreview" class="p-1 text-neutral-300 hover:bg-neutral-600 rounded">
                <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24"
                  stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Output console -->
          <pre v-if="selectedLanguage !== 'html'" ref="outputRef"
            class="p-3 overflow-auto text-sm flex-1 font-mono whitespace-pre-wrap break-words text-left">{{ output }}</pre>

          <!-- HTML Preview -->
          <div v-else class="flex-1 h-full relative overflow-hidden bg-white">
            <iframe :key="htmlPreviewKey" ref="htmlPreviewRef" class="w-full h-full border-none bg-white"
              sandbox="allow-scripts"></iframe>
          </div>

          <!-- Status messages -->
          <div v-if="isLoadingWasm" class="p-2 text-neutral-400 text-sm italic">Loading Wasm Engine...</div>
          <div v-if="wasmError" class="p-2 bg-red-900 text-white text-sm rounded-b">
            {{ wasmError }}
          </div>
          <div v-if="isRunning" class="p-2 text-neutral-400 text-sm italic">Running...</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, onMounted, computed, nextTick, onUnmounted, watch } from 'vue';
import { Codemirror } from 'vue-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { html } from '@codemirror/lang-html';
import { python } from '@codemirror/lang-python';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { getQuickJS, QuickJSContext } from 'quickjs-emscripten';
import { useCodeEditor } from '@/composables/useCodeEditor';

// Use the global code editor composable
const { code, isEditorOpen } = useCodeEditor();

// --- Refs ---
const output = ref<string>('');
const isRunning = ref<boolean>(false);
const isLoadingWasm = ref<boolean>(true);
const wasmError = ref<string | null>(null);
const quickJSVm = shallowRef<QuickJSContext | null>(null); // Use shallowRef for complex non-reactive objects
const outputRef = ref<HTMLPreElement | null>(null);
const htmlPreviewRef = ref<HTMLIFrameElement | null>(null);
const cmView = shallowRef<EditorView>(); // To access CodeMirror view instance if needed
const selectedLanguage = ref<'javascript' | 'python' | 'html'>('javascript');
const htmlPreviewKey   = ref<number>(0);
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
  python: "print('Hello from Python!')",
  html: htmlCode.value
};

// --- CodeMirror ---
const cmTheme = oneDark; // Make this selectable later if needed
const cmExtensions = computed(() => {
  let langSupport;
  switch (selectedLanguage.value) {
    case 'python':
      langSupport = python();
      break;
    case 'html':
      langSupport = html();
      break;
    case 'javascript':
    default:
      langSupport = javascript();
      break;
  }

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
const handleLanguageChange = (evt: Event) => {
  const oldLang = (evt.target as HTMLSelectElement).value as keyof typeof codeStore;
  codeStore[oldLang] = code.value;                  // keep outgoing buffer
  code.value = codeStore[selectedLanguage.value];   // load incoming buffer
  if (selectedLanguage.value === 'html') renderHtml(true);
};

// Watch for language changes to update the placeholder and editor
watch(selectedLanguage, (newLang) => {
  // When switching to HTML, render it automatically
  if (newLang === 'html') {
    nextTick(renderHtml);
  }
});

watch(htmlPreviewRef, (newIframe) => {
  console.log(`[${new Date().toISOString()}] htmlPreviewRef watcher triggered. New value: ${newIframe ? 'iframe element' : newIframe}`);
  // Only render if the iframe just became available AND we are in HTML mode
  if (newIframe && selectedLanguage.value === 'html') {
    console.log(`[${new Date().toISOString()}] Ref became available while in HTML mode. Triggering renderHtml.`);
    // Optional: Debounce or add a small delay if needed
    // Use nextTick to ensure any related state updates are processed
    nextTick(() => {
        renderHtml();
    });
  }
});

// HTML rendering function
const renderHtml = (force = false) => {
  if (force) htmlPreviewKey.value += 1;             // guarantees reload
  const iframe = htmlPreviewRef.value;
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

const runCode = async () => {
  if (selectedLanguage.value === 'html') {
    await nextTick();      // wait for last keystroke commit
    renderHtml(true);
    return;
  }
  
  isRunning.value = true;
  output.value = `Executing at ${new Date().toLocaleTimeString()}...\n\n`;
  scrollToOutputBottom();
  
  // Handle Python code differently - call backend API
  if (selectedLanguage.value === 'python') {
    executePython();
    return;
  }

  // JavaScript execution using QuickJS
  if (!quickJSVm.value) return;

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

// Python execution function
const executePython = async () => {
  try {
    output.value += 'Executing Python code...\n\n';
    
    // Call the backend API to execute Python code
    const response = await fetch('http://localhost:8080/api/code/eval', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        code: code.value,
        language: 'python',
      })
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`API error (${response.status}): ${errorText}`);
    }
    
    const data = await response.json();
    
    // Show the execution result
    if (data.error) {
      output.value += `\n--- EXECUTION ERROR ---\n${data.error}\n`;
    } else {
      output.value += `\n--- EXECUTION SUCCESS ---\n${data.result || 'No output'}\n`;
    }
  } catch (error: any) {
    output.value += `\n--- ERROR ---\n${error.message || 'An unexpected error occurred'}\n`;
    console.error('Error executing Python code:', error);
  } finally {
    isRunning.value = false;
    scrollToOutputBottom();
  }
};
</script>

<!-- No scoped styles; Tailwind classes are used instead -->