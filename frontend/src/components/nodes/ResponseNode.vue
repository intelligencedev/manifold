<template>
  <BaseNode :id="id" :data="data" :min-height="180" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold">
        {{ modelTypeLabel }}
      </div>
    </template>

    <div class="flex flex-col h-full w-full">
      <div class="controls flex flex-wrap gap-2 items-center mb-2">
        <BaseSelect :id="`${data.id}-model-type`" label="Model" v-model="selectedModelType" :options="modelOptions" />
        <BaseSelect :id="`${data.id}-render-mode`" label="Render Mode" v-model="selectedRenderMode" :options="renderModeOptions" />
        <BaseSelect v-if="selectedRenderMode === 'markdown'" :id="`${data.id}-theme`" label="Theme" v-model="selectedTheme" :options="themeOptions" />
        <div class="font-size-controls flex gap-1 items-center">
          <button @click.prevent="decreaseFontSize" class="px-2 py-1 rounded bg-purple-700 hover:bg-purple-800 text-xs">-</button>
          <button @click.prevent="increaseFontSize" class="px-2 py-1 rounded bg-purple-700 hover:bg-purple-800 text-xs">+</button>
        </div>
        <button class="copy-button px-3 py-1 rounded bg-purple-600 hover:bg-purple-700 text-white text-xs" @click="copyToClipboard" :disabled="isCopying">
          Copy
        </button>
      </div>
      <div v-if="copyStatus" class="copy-feedback text-xs text-green-400 mb-2">{{ copyStatus }}</div>

      <div
        class="flex-1 h-full w-full text-left overflow-auto rounded bg-zinc-800 p-2"
        ref="textContainer"
        @scroll="handleScroll"
        @mouseenter="$emit('disable-zoom')"
        @mouseleave="$emit('enable-zoom')"
        :style="{ fontSize: `${currentFontSize}px` }"
      >
        <template v-for="(t, idx) in thinkingBlocks" :key="idx">
          <div class="think-wrapper" :data-collapsed="t.collapsed" @click.stop="toggleThink(idx)">
            <div class="think-header">
              <span class="think-icon">💭</span>
              <span class="think-title">Agent Thinking</span>
            </div>
            <pre v-if="t.collapsed" class="think-preview">{{ t.preview }}</pre>
            <pre v-else class="think-content">{{ t.content }}</pre>
            <div v-if="t.hasMore" class="think-toggle">
              <span v-if="t.collapsed" class="chevron-down">▼</span>
              <span v-else class="chevron-up">▲</span>
            </div>
          </div>
        </template>

        <div v-if="selectedRenderMode === 'markdown'" class="markdown-text" v-html="markdownOutsideThinking" />
        <pre v-else-if="selectedRenderMode === 'raw'" class="raw-text">{{ outsideThinkingRaw }}</pre>
        <div v-else-if="selectedRenderMode === 'html'" class="html-content" v-html="htmlOutsideThinking" />
      </div>

      <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
      <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
    </div>
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import { useResponseNode } from '@/composables/useResponseNode'

const props = defineProps({
  id: { type: String, required: true, default: 'Response_0' },
  data: {
    type: Object,
    default: () => ({
      type: 'ResponseNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: { response: '' },
      outputs: {},
    })
  }
})

const emit = defineEmits(['update:data','disable-zoom','enable-zoom','resize'])

const renderModeOptions = [
  { value: 'markdown', label: 'Markdown' },
  { value: 'raw', label: 'Raw Text' },
  { value: 'html', label: 'HTML' }
]

const modelOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' }
]

const themeOptions = [
  { value: 'atom-one-dark', label: 'Dark' },
  { value: 'atom-one-light', label: 'Light' },
  { value: 'github', label: 'GitHub' },
  { value: 'monokai', label: 'Monokai' },
  { value: 'vs', label: 'VS' }
]

const {
  isHovered,
  resizeHandleStyle,
  computedContainerStyle,
  onResize,
  selectedTheme,
  selectedModelType,
  selectedRenderMode,
  modelTypeLabel,
  currentFontSize,
  increaseFontSize,
  decreaseFontSize,
  copyStatus,
  isCopying,
  copyToClipboard,
  textContainer,
  handleScroll,
  markdownOutsideThinking,
  htmlOutsideThinking,
  outsideThinkingRaw,
  thinkingBlocks,
  toggleThink
} = useResponseNode(props, emit)
</script>

<style scoped>
/* -------- thinking block -------- */
.think-wrapper {
  font-style: italic;
  color: #d8d0e8;
  background: rgba(73,49,99,.25);
  border-left: 3px solid #8a70b5;
  margin: 12px 0;
  border-radius: 8px;
  overflow: hidden;
  position: relative;
  cursor: pointer;
}
.think-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-bottom: 1px solid rgba(138,112,181,.3);
}
.think-title { font-weight: 600; color:#b899e0; }
.think-preview, .think-content {
  white-space: pre-wrap;
  padding: 10px;
  background: rgba(40,30,55,.2);
  margin: 0;
}
.think-content { background: rgba(45,35,65,.3); }
.think-toggle {
  text-align: center;
  cursor: pointer;
  font-size: 12px;
  padding: 3px 0 5px;
  background: rgba(73,49,99,.3);
  user-select: none;
}
.think-wrapper[data-collapsed="false"] .think-preview { display:none; }
.think-wrapper[data-collapsed="true"]  .think-content { display:none; }

/* -------- thinking block preview line clamping -------- */
.think-preview {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-height: calc(1.2em * 2);
}
</style>
