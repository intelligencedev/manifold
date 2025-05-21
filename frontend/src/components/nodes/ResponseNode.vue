<template>
  <BaseNode :id="id" :data="data" :min-height="180" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold">
        {{ modelTypeLabel }}
      </div>
    </template>

    <div class="flex flex-col gap-2 mb-2">
      <div class="controls flex flex-wrap gap-2 items-center">
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
      <slot />
    </div>

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
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
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '624px',
        height: '400px'
      }
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
  handleScroll
} = useResponseNode(props, emit)
</script>

<!-- Styling handled by Tailwind and Base components -->
