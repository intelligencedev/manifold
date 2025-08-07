<template>
    <div class="wasm-code-editor-container" style="text-align: left;">
      <!-- Toolbar with language selector -->
      <div class="toolbar">
        <div class="language-selector">
          <label for="lang-select">Language:</label>
          <select id="lang-select" v-model="selectedLanguage" @change="handleLanguageChange">
            <option value="javascript">JavaScript</option>
            <option value="python">Python</option>
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
          :placeholder="selectedLanguage === 'html' ? 'Enter HTML code...' : 'Enter JavaScript code...'"
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
        <div v-else ref="htmlPreviewRef" class="html-preview"></div>
      </div>
  
      <!-- Status/Loading -->
      <div v-if="isLoadingWasm" class="status-message">Loading Wasm Engine...</div>
      <div v-if="wasmError" class="error-message">Wasm Error: {{ wasmError }}</div>
      <div v-if="isRunning" class="status-message">Running...</div>
  
    </div>
  </template>
  
  <script setup lang="ts">
  import { ref, shallowRef, onMounted, computed, nextTick, watch, onUnmounted } from 'vue';
  import { Codemirror } from 'vue-codemirror';
  import { javascript } from '@codemirror/lang-javascript';
  import { html } from '@codemirror/lang-html';
  import { python } from '@codemirror/lang-python';
  import { oneDark } from '@codemirror/theme-one-dark'; // Example theme
  import { EditorView } from '@codemirror/view';
  import { getQuickJS, QuickJSContext } from 'quickjs-emscripten';
  import { useConfigStore } from '@/stores/configStore';
  import { getApiEndpoint, API_PATHS } from '@/utils/endpoints';
  
  // --- Refs ---
  const code = ref<string>('console.log("Hello from Manifold!");');
  const output = ref<string>('');
  const isRunning = ref<boolean>(false);
  const isLoadingWasm = ref<boolean>(true);
  const wasmError = ref<string | null>(null);
  const quickJSVm = shallowRef<QuickJSContext | null>(null); // Use shallowRef for complex non-reactive objects
  const configStore = useConfigStore();
  const outputRef = ref<HTMLPreElement | null>(null);
  const htmlPreviewRef = ref<HTMLDivElement | null>(null);
  const cmView = shallowRef<EditorView>(); // To access CodeMirror view instance if needed
  const selectedLanguage = ref<'javascript' | 'python' | 'html'>('javascript');
  // Add missing height refs for resizable panels
  const editorHeightPercent = ref<number>(50); // Initial height for editor
  const savedCode = {
    javascript: 'console.log("Hello from Manifold!");',
    python: 'print("Hello from Python!")',
    html: '<html>\n  <head>\n    <style>\n      body {\n        font-family: sans-serif;\n        padding: 20px;\n      }\n      h1 {\n        color: #336699;\n      }\n    </style>\n  </head>\n  <body>\n    <h1>Hello HTML World!</h1>\n    <p>Edit this HTML to see it rendered in the preview panel.</p>\n  </body>\n</html>'
  };
  
  // --- CodeMirror ---
  const cmTheme = oneDark; // Make this selectable later if needed
  const cmExtensions = computed(() => {
    const langSupport = selectedLanguage.value === 'html' ? html() : selectedLanguage.value === 'python' ? python() : javascript();
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
  
    // Initialize HTML preview if language is set to HTML
    if (selectedLanguage.value === 'html' && htmlPreviewRef.value) {
      htmlPreviewRef.value.innerHTML = code.value;
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
  
  // --- HTML Preview ---
  watch(selectedLanguage, (newLang, oldLang) => {
    // Save current code in the appropriate language store
    savedCode[oldLang] = code.value;
    
    // Load code for the newly selected language
    code.value = savedCode[newLang];
    
    // If switching to HTML, render the preview
    if (newLang === 'html') {
      nextTick(() => {
        if (htmlPreviewRef.value) {
          htmlPreviewRef.value.innerHTML = code.value;
        }
      });
    }
  });
  
  const handleLanguageChange = () => {
    // The watch on selectedLanguage will handle the switch
  };
  
  // --- Core Logic ---
  const runCode = () => {
    if (selectedLanguage.value === 'html') {
      renderHtml();
      return;
    }
    
    // Handle Python code execution
    if (selectedLanguage.value === 'python') {
      executePython();
      return;
    }
  
    if (!quickJSVm.value || isRunning.value) return;
  
    isRunning.value = true;
    output.value = `Executing at ${new Date().toLocaleTimeString()}...\n\n`;
    scrollToOutputBottom();
  
    // Use setTimeout to allow UI update before potentially blocking execution
    setTimeout(() => {
      try {
        const vm = quickJSVm.value as QuickJSContext; // Type assertion
  
        // Capture console.log, console.error, etc.
        // Expose functions from JS host (Vue) to Wasm guest (QuickJS)
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
  
  // Python execution function
  const executePython = async () => {
    try {
      isRunning.value = true;
      output.value = `Executing Python code at ${new Date().toLocaleTimeString()}...\n\n`;
      
      // Call the backend API to execute Python code
      const response = await fetch(getApiEndpoint(configStore.config, API_PATHS.CODE_EVAL), {
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
  
  const renderHtml = () => {
    if (!htmlPreviewRef.value) return;
    
    try {
      // Render the HTML in the preview panel
      htmlPreviewRef.value.innerHTML = code.value;
    } catch (error: any) {
      console.error('Error rendering HTML:', error);
      htmlPreviewRef.value.innerHTML = `
        <div class="html-render-error">
          <h3>Error Rendering HTML</h3>
          <p>${error.message || 'An unknown error occurred'}</p>
        </div>
      `;
    }
  };
  
  const refreshPreview = () => {
    if (selectedLanguage.value === 'html') {
      renderHtml();
    }
  };
  
  const clearOutput = () => {
    if (selectedLanguage.value === 'html' && htmlPreviewRef.value) {
      htmlPreviewRef.value.innerHTML = '';
    } else {
      output.value = '';
    }
  };
  
  const scrollToOutputBottom = () => {
    nextTick(() => {
      if (outputRef.value) {
        outputRef.value.scrollTop = outputRef.value.scrollHeight;
      }
    });
  };
  
  // --- Resizing Logic ---
  let isResizing = false;

  const startResize = (event: MouseEvent | TouchEvent) => {
    isResizing = true;
    
    // Immediately set initial divider position based on mouse position
    handleMouseMove(event);
    
    // Add event listeners
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('touchmove', handleMouseMove, { passive: false });
    document.addEventListener('mouseup', stopResize);
    document.addEventListener('touchend', stopResize);
    
    // Prevent default to avoid text selection
    event.preventDefault();
  };

  const handleMouseMove = (event: MouseEvent | TouchEvent) => {
    if (!isResizing) return;
    
    // Get container information for bounds checking
    const container = document.querySelector('.wasm-code-editor-container');
    if (!container) return;
    
    const containerRect = container.getBoundingClientRect();
    const containerTop = containerRect.top;
    const containerHeight = containerRect.height;
    
    // Get current mouse position
    const clientY = event instanceof MouseEvent ? event.clientY : event.touches[0].clientY;
    
    // Calculate divider position relative to container
    const dividerPositionInContainer = Math.max(100, Math.min(containerHeight - 100, clientY - containerTop));
    
    // Set panel heights directly based on divider position
    editorHeightPercent.value = (dividerPositionInContainer / containerHeight) * 100;
    
    // Prevent scrolling on touch devices
    if (event instanceof TouchEvent) {
      event.preventDefault();
    }
  };

  const stopResize = () => {
    isResizing = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('touchmove', handleMouseMove);
    document.removeEventListener('mouseup', stopResize);
    document.removeEventListener('touchend', stopResize);
  };
  </script>
  
  <style scoped>
  .wasm-code-editor-container {
    display: flex;
    flex-direction: column;
    height: 100%; /* Fill parent (UtilityPalette) */
    background-color: #2a2a2a; /* Match palette */
    color: #eee;
    font-family: sans-serif;
    padding: 10px;
    box-sizing: border-box;
    text-align: left; /* Override global centering */
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
  
  .output-area h3 {
    margin: 0;
    padding: 8px 0;
    font-size: 0.9em;
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
  }
  
  .html-preview {
    flex-grow: 1;
    overflow: auto;
    background-color: white;
    color: black;
    padding: 0;
    height: 100%;
  }
  
  /* Content within the iframe should be styled by the HTML itself */
  .html-preview :deep(html),
  .html-preview :deep(body) {
    margin: 0;
    padding: 0;
    height: 100%;
  }
  
  .html-render-error {
    padding: 20px;
    color: #ff6b6b;
    background-color: rgba(255, 107, 107, 0.1);
    border: 1px solid #ff6b6b;
    border-radius: 4px;
    margin: 10px;
  }
  
  .output-content::-webkit-scrollbar,
  .html-preview::-webkit-scrollbar {
    width: 6px;
  }
  .output-content::-webkit-scrollbar-thumb,
  .html-preview::-webkit-scrollbar-thumb {
    background-color: #555;
    border-radius: 3px;
  }
  .output-content::-webkit-scrollbar-track,
  .html-preview::-webkit-scrollbar-track {
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
  
  .resize-handle {
    cursor: ns-resize;
    height: 10px;
    background-color: transparent;
    margin: 0 10px;
    position: relative;
  }
  
  .handle-indicator {
    position: absolute;
    top: -5px;
    left: 50%;
    transform: translateX(-50%);
    width: 20px;
    height: 20px;
    background-color: #444;
    border-radius: 10px;
  }
  </style>
