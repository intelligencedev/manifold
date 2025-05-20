<template>
  <div
    :style="computedContainerStyle"
    class="node-container mlxflux-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-pink-400 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseAccordion title="Parameters" :initiallyOpen="true">
      <div class="flex flex-col gap-2">
        <BaseInput :id="`${data.id}-model`" label="Model" v-model="model" class="mb-1" />
        <BaseTextarea :id="`${data.id}-prompt`" label="Prompt" v-model="prompt" class="mb-1" />
        <BaseInput :id="`${data.id}-steps`" label="Steps" v-model.number="steps" type="number" min="1" class="mb-1" />
        <BaseInput :id="`${data.id}-seed`" label="Seed" v-model.number="seed" type="number" class="mb-1" />
        <div class="flex items-center gap-2 mb-1">
          <span class="input-label text-xs">Quality:</span>
          <label class="radio-label flex items-center gap-1">
            <input type="radio" :name="`${data.id}-quality`" :value="4" v-model.number="quality" /> 4bit
          </label>
          <label class="radio-label flex items-center gap-1">
            <input type="radio" :name="`${data.id}-quality`" :value="8" v-model.number="quality" /> 8bit
          </label>
        </div>
        <BaseInput :id="`${data.id}-output`" label="File Name" v-model="output" class="mb-1" />
      </div>
    </BaseAccordion>

    <div class="generated-image-panel flex-1 flex items-center justify-center mt-2" v-if="imageSrc">
      <img :src="imageSrc" alt="Generated Image" class="generated-image max-w-full max-h-48 rounded border border-gray-700" />
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <NodeResizer
      :is-resizable="true"
      :color="'#ec4899'"
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
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useMLXFluxNode } from '@/composables/useMLXFluxNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'MLXFlux_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'MLXFlux',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        model: '',
        prompt: '',
        steps: 20,
        seed: 0,
        quality: 8,
        output: '',
      },
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '180px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const vueFlowInstance = useVueFlow()
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  model,
  prompt,
  steps,
  seed,
  quality,
  output,
  imageSrc,
  onResize
} = useMLXFluxNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>
