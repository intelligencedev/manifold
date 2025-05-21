<template>
  <div
    :style="computedContainerStyle"
    class="node-container glsl-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <div class="glsl-canvas-container flex-1 flex items-center justify-center mb-2">
      <canvas ref="shaderCanvas" class="shader-canvas w-full h-48 rounded bg-black border border-gray-700"></canvas>
    </div>

    <BaseAccordion v-if="data.showEditor" title="Shader Editor" :initiallyOpen="false">
      <BaseTextarea 
        label="Fragment Shader" 
        v-model="fragmentShader"
        class="shader-textarea mb-2"
      />
      <button @click="run" class="px-3 py-1 rounded bg-blue-600 hover:bg-blue-700 text-white text-sm">Run Shader</button>
    </BaseAccordion>

    <Handle 
      v-if="data.hasInputs"
      style="width:12px; height:12px" 
      type="target" 
      position="left" 
    />
    <Handle 
      v-if="data.hasOutputs"
      style="width:12px; height:12px" 
      type="source" 
      position="right" 
    />

    <NodeResizer 
      :is-resizable="true" 
      :color="'#666'" 
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle" 
      :min-width="320" 
      :min-height="240"
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useGLSLNode } from '@/composables/useGLSLNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'GLSLNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'GLSLNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      showEditor: true,
      inputs: {},
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#222',
        color: '#eee',
        width: '360px',
        height: '320px',
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
  fragmentShader,
  run,
  onResize
} = useGLSLNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>