<template>
  <BaseNode :id="id" :data="data" :min-height="300" :min-width="504" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle">
        {{ modelTypeLabel }}
      </div>
    </template>

    <div class="flex flex-col h-full w-full">
      <div class="controls flex flex-wrap gap-2 items-center mb-2">
        <BaseDropdown :id="`${data.id}-model-type`" label="Model" v-model="selectedModelType" :options="modelOptions" />
        <BaseDropdown :id="`${data.id}-render-mode`" label="Render Mode" v-model="selectedRenderMode" :options="renderModeOptions" />
        <BaseDropdown v-if="selectedRenderMode === 'markdown'" :id="`${data.id}-theme`" label="Theme" v-model="selectedTheme" :options="themeOptions" />
        <div class="flex-1"></div>
        <div class="font-size-controls flex gap-2 items-center">
          <BaseButton @click.prevent="decreaseFontSize" class="bg-slate-600 hover:bg-slate-800 text-xs">-</BaseButton>
          <BaseButton @click.prevent="increaseFontSize" class="bg-slate-600 hover:bg-slate-800 text-xs">+</BaseButton>
        </div>
        <BaseButton @click="copyToClipboard" :disabled="isCopying" class="bg-slate-600 hover:bg-slate-800 text-white text-lg">Copy</BaseButton>
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
        <div v-if="thinkingBlocks.length" :key="reRenderKey" class="think-wrapper">
          <pre class="think-content">{{ thinkingBlocks[0].content }}</pre>
        </div>

        <div v-if="selectedRenderMode === 'markdown'" class="text-white tracking-wide" v-html="markdownOutsideThinking" />
        <pre v-else-if="selectedRenderMode === 'raw'" class="text-white tracking-wide">{{ outsideThinkingRaw }}</pre>
        <div v-else-if="selectedRenderMode === 'html'" class="text-white tracking-wide" v-html="htmlOutsideThinking" />
      </div>

      <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
      <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
    </div>
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseDropdown from '@/components/base/BaseDropdown.vue'
import BaseButton from '@/components/base/BaseButton.vue'
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
  reRenderKey
} = useResponseNode(props, emit)
</script>

<style scoped>
/* -------- thinking block -------- */
.think-wrapper {
  color: #d8d0e8;
  background: rgba(73,49,99,.25);
  border-left: 3px solid #8a70b5;
  margin: 12px 0;
  border-radius: 8px;
  overflow: hidden;
  position: relative;
}
.think-content {
  white-space: pre-wrap;
  padding: 10px;
  background: rgba(45,35,65,.3);
  margin: 0;
}


/* Removed fade-in transition, no animation for thinking block */


/* Markdown styling for headers and other elements */
:deep(h1) {
  font-size: 1.9em;
  font-weight: bold;
  margin: 0.67em 0;
}
:deep(h2) {
  font-size: 1.5em;
  font-weight: bold;
  margin: 0.83em 0;
}
:deep(h3) {
  font-size: 1.3em;
  font-weight: bold;
  margin: 1em 0;
}
:deep(h4) {
  font-size: 1.1em;
  font-weight: bold;
  margin: 1.33em 0;
}
:deep(h5) {
  font-size: 1em;
  font-weight: bold;
  margin: 1.5em 0;
}
:deep(h6) {
  font-size: 0.9em;
  font-weight: bold;
  margin: 1.67em 0;
}
:deep(p) {
  margin: 1em 0;
}
:deep(ul), :deep(ol) {
  padding-left: 1.5em;
  margin: 1em 0;
}
:deep(ul) {
  list-style-type: disc;
}
:deep(ol) {
  list-style-type: decimal;
}
:deep(li) {
  margin: 0.5em 0;
}
:deep(pre) {
  margin: 1em 0;
  padding: 1em;
  background-color: rgba(45, 45, 45, 0.5);
  border-radius: 0.25em;
  overflow-x: auto;
}
:deep(code) {
  font-family: monospace;
  background-color: rgba(45, 45, 45, 0.3);
  padding: 0.2em 0.4em;
  border-radius: 0.2em;
}
:deep(pre code) {
  background-color: transparent;
  padding: 0;
  border-radius: 0;
}
:deep(blockquote) {
  margin: 1em 0;
  padding-left: 1em;
  border-left: 4px solid #4a5568;
  color: #a0aec0;
}
:deep(table) {
  border-collapse: collapse;
  margin: 1em 0;
  width: 100%;
}
:deep(th), :deep(td) {
  border: 1px solid #4a5568;
  padding: 0.5em;
  text-align: left;
}
:deep(th) {
  background-color: rgba(45, 45, 45, 0.3);
}
</style>
