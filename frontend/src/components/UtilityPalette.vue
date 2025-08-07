<template>
  <!-- Slide-out palette -->
  <div
    :class="[
      'fixed top-[64px] bottom-0 right-0 w-1/2 z-[1100] flex flex-col text-white transition-transform duration-300 shadow-[-5px_0_10px_-5px_rgba(0,0,0,0.3)] dark:bg-zinc-900',
      isEditorOpen ? 'translate-x-0' : 'translate-x-full'
    ]"
    style="text-align: left;"
  >
    <!-- Toggle handle -->
    <div
      class="absolute top-1/2 left-0 -translate-x-full w-[30px] h-[60px] rounded-l-md flex items-center justify-center cursor-pointer dark:bg-zinc-900 shadow-[-5px_0_10px_-5px_rgba(0,0,0,0.3)]"
      @click="togglePalette"
    >
      <svg
        v-if="isEditorOpen"
        xmlns="http://www.w3.org/2000/svg"
        class="w-5 h-5"
        viewBox="0 0 16 16"
        fill="currentColor"
      >
        <path
          fill-rule="evenodd"
          d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z"
        />
      </svg>
      <svg
        v-else
        xmlns="http://www.w3.org/2000/svg"
        class="w-5 h-5"
        viewBox="0 0 16 16"
        fill="currentColor"
      >
        <path
          fill-rule="evenodd"
          d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z"
        />
      </svg>
    </div>

    <!-- Body -->
    <div class="p-4 h-full flex flex-col box-border">
      <div class="flex-1 flex flex-col gap-4 min-h-0">
        <!-- Toolbar -->
        <div
          class="flex justify-between items-center flex-wrap gap-2 pb-3 border-b border-neutral-600"
        >
          <div class="flex items-center gap-2">
            <label for="lang-select" class="text-sm">Language:</label>
            <select
              id="lang-select"
              v-model="selectedLanguage"
              @change="handleLanguageChange"
              class="p-1 bg-neutral-700 text-white border border-neutral-600 rounded text-sm"
            >
              <option value="javascript">JavaScript</option>
              <option value="python">Python</option>
              <option value="html">HTML</option>
            </select>
          </div>
          <div class="flex gap-2">
            <button
              @click="runCode"
              :disabled="isRunning || !quickJSVm"
              class="px-3 py-1 bg-teal-700 hover:bg-teal-600 text-white rounded disabled:opacity-50 disabled:cursor-not-allowed text-sm"
            >
              {{ selectedLanguage === 'html' ? 'Render' : 'Run' }}
            </button>
            <button
              @click="clearOutput"
              class="px-3 py-1 bg-neutral-700 text-white rounded hover:bg-neutral-600 text-sm"
            >
              Clear Output
            </button>
          </div>
        </div>

        <!-- Resizable editor / output -->
        <div
          ref="resizeContainer"
          class="flex-1 flex flex-col gap-1 min-h-0 resizable-panels-wrapper"
        >
          <!-- Editor -->
          <div
            class="min-h-[120px] border border-neutral-600 rounded overflow-hidden flex-none"
            :style="{ flexBasis: editorHeightPercent + '%' }"
          >
            <Codemirror
              v-model="code"
              placeholder="Enter code…"
              class="h-full"
              :autofocus="true"
              :indent-with-tab="true"
              :tab-size="2"
              :extensions="cmExtensions"
              @ready="handleCmReady"
            />
          </div>

          <!-- Divider -->
          <div
            class="h-4 cursor-ns-resize flex items-center justify-center relative resizable-divider select-none"
            @mousedown="startResize"
            @touchstart="startResize"
          >
            <div class="w-12 h-1 rounded-full bg-neutral-500"></div>
          </div>

          <!-- Output -->
          <div
            class="min-h-[100px] border border-neutral-600 rounded flex flex-col bg-neutral-900 overflow-hidden flex-none"
            :style="{ flexBasis: 100 - editorHeightPercent + '%' }"
          >
            <div
              class="flex justify-between items-center bg-neutral-700 border-b border-neutral-600 px-3 py-2"
            >
              <h3 class="text-sm font-medium">
                {{ selectedLanguage === 'html' ? 'HTML Preview' : 'Output / Logs' }}
              </h3>
              <button
                v-if="selectedLanguage === 'html'"
                @click="refreshPreview"
                class="p-1 text-neutral-300 hover:bg-neutral-600 rounded"
              >
                ↻
              </button>
            </div>

            <!-- Console -->
            <pre
              v-if="selectedLanguage !== 'html'"
              ref="outputRef"
              class="flex-1 p-3 overflow-auto text-sm font-mono whitespace-pre-wrap break-words text-left"
            >
{{ output }}
            </pre>

            <!-- HTML preview -->
            <div v-else class="flex-1 bg-white relative">
              <iframe
                :key="htmlPreviewKey"
                ref="htmlPreviewRef"
                class="w-full h-full border-none bg-white"
                sandbox="allow-scripts"
              ></iframe>
            </div>

            <!-- Status -->
            <div
              v-if="isLoadingWasm"
              class="p-2 text-neutral-400 text-sm italic"
            >
              Loading Wasm Engine…
            </div>
            <div
              v-if="wasmError"
              class="p-2 bg-red-900 text-white text-sm"
            >
              {{ wasmError }}
            </div>
            <div
              v-if="isRunning"
              class="p-2 text-neutral-400 text-sm italic"
            >
              Running…
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/* ———————————————————————————————————————————————————————————
   imports & reactive state
   ——————————————————————————————————————————————————————————— */
import {
  ref,
  shallowRef,
  onMounted,
  computed,
  nextTick,
  onUnmounted,
  watch,
} from 'vue';
import { Codemirror } from 'vue-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { html } from '@codemirror/lang-html';
import { python } from '@codemirror/lang-python';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { getQuickJS, QuickJSContext } from 'quickjs-emscripten';
import { useCodeEditor } from '@/composables/useCodeEditor';
import { useConfigStore } from '@/stores/configStore';
import { getApiEndpoint, API_PATHS } from '@/utils/endpoints';

const { code, isEditorOpen } = useCodeEditor();
const configStore = useConfigStore();

/* ———————————————————————————————————————————————————————————
   refs & misc
   ——————————————————————————————————————————————————————————— */
const output = ref('');
const isRunning = ref(false);
const isLoadingWasm = ref(true);
const wasmError = ref<string | null>(null);
const quickJSVm = shallowRef<QuickJSContext | null>(null);
const outputRef = ref<HTMLPreElement | null>(null);
const htmlPreviewRef = ref<HTMLIFrameElement | null>(null);
const resizeContainer = ref<HTMLDivElement | null>(null);

const cmView = shallowRef<EditorView>();
const selectedLanguage = ref<'javascript' | 'python' | 'html'>('javascript');
const htmlPreviewKey = ref(0);

const editorHeightPercent = ref(50);

/* ———————————————————————————————————————————————————————————
   Code-store per language
   ——————————————————————————————————————————————————————————— */
const htmlStarter = `<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: Arial, sans-serif; padding: 20px; }
    h1 { color: #336699; }
  </style>
</head>
<body>
  <h1>Hello, HTML preview!</h1>
  <p>Edit the code and click “Render”.</p>
</body>
</html>`;
const codeStore: Record<'javascript' | 'python' | 'html', string> = {
  javascript: "console.log('Hello from Manifold!');",
  python: "print('Hello from Python!')",
  html: htmlStarter,
};

/* ———————————————————————————————————————————————————————————
   CodeMirror
   ——————————————————————————————————————————————————————————— */
const cmTheme = oneDark;
const cmExtensions = computed(() => {
  const lang = {
    javascript,
    python,
    html,
  }[selectedLanguage.value]();
  return [
    lang,
    cmTheme,
    EditorView.lineWrapping,
    EditorView.theme({
      '&': { fontSize: '13px', height: '100%' },
      '.cm-scroller': { overflow: 'auto' },
    }),
  ];
});

const handleCmReady = (e: any) => (cmView.value = e.view);

/* ———————————————————————————————————————————————————————————
   QuickJS init / cleanup
   ——————————————————————————————————————————————————————————— */
onMounted(async () => {
  try {
    const QuickJS = await getQuickJS();
    quickJSVm.value = QuickJS.newContext();
  } catch (e: any) {
    wasmError.value = e.message ?? 'Wasm load error';
  } finally {
    isLoadingWasm.value = false;
  }
});
onUnmounted(() => quickJSVm.value?.dispose());

/* ———————————————————————————————————————————————————————————
   palette visibility toggle
   ——————————————————————————————————————————————————————————— */
function togglePalette() {
  isEditorOpen.value = !isEditorOpen.value;
}

/* ———————————————————————————————————————————————————————————
   language switch
   ——————————————————————————————————————————————————————————— */
function handleLanguageChange(evt: Event) {
  const old = (evt.target as HTMLSelectElement).value as keyof typeof codeStore;
  codeStore[old] = code.value;
  code.value = codeStore[selectedLanguage.value];
  if (selectedLanguage.value === 'html') renderHtml(true);
}
watch(selectedLanguage, (l) => l === 'html' && nextTick(renderHtml));

/* ———————————————————————————————————————————————————————————
   HTML preview
   ——————————————————————————————————————————————————————————— */
function renderHtml(force = false) {
  if (force) htmlPreviewKey.value++;
  const iframe = htmlPreviewRef.value;
  if (!iframe) return;
  iframe.srcdoc = code.value;
}
function refreshPreview() {
  if (selectedLanguage.value === 'html') renderHtml(true);
}
watch(htmlPreviewRef, (i) => i && selectedLanguage.value === 'html' && nextTick(renderHtml));

/* ———————————————————————————————————————————————————————————
   run / clear
   ——————————————————————————————————————————————————————————— */
async function runCode() {
  if (selectedLanguage.value === 'html') {
    await nextTick();
    renderHtml(true);
    return;
  }

  isRunning.value = true;
  output.value = `Executing at ${new Date().toLocaleTimeString()}…\n\n`;
  scrollToBottom();

  if (selectedLanguage.value === 'python') {
    await executePython();
    return;
  }

  if (!quickJSVm.value) return;

  setTimeout(() => {
    try {
      const vm = quickJSVm.value!;
      const logger = (tag: string) =>
        vm.newFunction(tag, (...args) => {
          output.value += `[${tag.toUpperCase()}] ${args.map(vm.dump).join(' ')}\n`;
          scrollToBottom();
        });

      const consoleObj = vm.newObject();
      ['log', 'error', 'warn'].forEach((m) =>
        vm.setProp(consoleObj, m, logger(m)),
      );
      vm.setProp(vm.global, 'console', consoleObj);

      const result = vm.evalCode(code.value);
      if (result.error) {
        output.value += `\n--- EXECUTION ERROR ---\n${vm.dump(result.error)}\n`;
        result.error.dispose();
      } else {
        output.value += `\n--- EXECUTION SUCCESS ---\n${vm.dump(result.value)}\n`;
        result.value.dispose();
      }
    } catch (e: any) {
      output.value += `\n--- HOST ERROR ---\n${e.message}\n`;
    } finally {
      isRunning.value = false;
      scrollToBottom();
    }
  }, 10);
}
function clearOutput() {
  output.value = '';
}
function scrollToBottom() {
  nextTick(() => outputRef.value && (outputRef.value.scrollTop = outputRef.value.scrollHeight));
}

/* ———————————————————————————————————————————————————————————
   Python (placeholder)
   ——————————————————————————————————————————————————————————— */
async function executePython() {
  try {
    const rsp = await fetch(getApiEndpoint(configStore.config, API_PATHS.CODE_EVAL), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ language: 'python', code: code.value }),
    });
    const data = await rsp.json();
    output.value += data.error
      ? `\n--- EXECUTION ERROR ---\n${data.error}\n`
      : `\n--- EXECUTION SUCCESS ---\n${data.result ?? 'No output'}\n`;
  } catch (e: any) {
    output.value += `\n--- ERROR ---\n${e.message}\n`;
  } finally {
    isRunning.value = false;
    scrollToBottom();
  }
}

/* ———————————————————————————————————————————————————————————
   drag-to-resize
   ——————————————————————————————————————————————————————————— */
let isResizing = false;
let startY = 0;
let frame: number | null = null;

function startResize(ev: MouseEvent | TouchEvent) {
  isResizing = true;
  startY = ev instanceof MouseEvent ? ev.clientY : ev.touches[0].clientY;
  document.addEventListener('mousemove', handleResize);
  document.addEventListener('touchmove', handleResize, { passive: false });
  document.addEventListener('mouseup', stopResize);
  document.addEventListener('touchend', stopResize);
  document.body.classList.add('select-none', 'cursor-ns-resize');
  ev.preventDefault();
}

function handleResize(ev: MouseEvent | TouchEvent) {
  if (!isResizing) return;

  const y = ev instanceof MouseEvent ? ev.clientY : ev.touches[0].clientY;
  const deltaY = y - startY;

  if (frame) cancelAnimationFrame(frame);
  frame = requestAnimationFrame(() => {
    const container = resizeContainer.value;
    if (!container) return;
    const { height } = container.getBoundingClientRect();
    const min = 100; // px
    const deltaPercent = (deltaY / height) * 100;
    const newPct = Math.min(
      100 - (min / height) * 100,
      Math.max((min / height) * 100, editorHeightPercent.value + deltaPercent),
    );
    editorHeightPercent.value = newPct;
    startY = y;
    cmView.value?.requestMeasure();
  });

  if (ev instanceof TouchEvent) ev.preventDefault();
}

function stopResize() {
  isResizing = false;
  document.removeEventListener('mousemove', handleResize);
  document.removeEventListener('touchmove', handleResize);
  document.removeEventListener('mouseup', stopResize);
  document.removeEventListener('touchend', stopResize);
  if (frame) {
    cancelAnimationFrame(frame);
    frame = null;
  }
  document.body.classList.remove('select-none', 'cursor-ns-resize');
}
</script>

<style>
.resizable-divider:hover {
  background-color: rgba(255, 255, 255, 0.1);
}
</style>
