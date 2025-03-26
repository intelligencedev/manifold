<template>
  <div 
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container glsl-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    
    <div class="glsl-canvas-container">
      <canvas ref="shaderCanvas" class="shader-canvas"></canvas>
    </div>
    
    <!-- Shader Editor (Optional) -->
    <BaseAccordion v-if="data.showEditor" title="Shader Editor" :initiallyOpen="false">
      <BaseTextarea 
        label="Fragment Shader" 
        v-model="fragmentShader"
        class="shader-textarea"
      />
      <button @click="run" class="run-shader-btn">Run Shader</button>
    </BaseAccordion>
    
    <!-- Input/Output Handles -->
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
    
    <!-- NodeResizer -->
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
    default: 'GLSLNode_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'GLSLNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: false,
      showEditor: true,
      inputs: {
        fragmentShader: `
precision mediump float;
varying vec2 vTextureCoord;
uniform float uTime;
uniform vec2 uResolution;

void main() {
  vec2 uv = vTextureCoord;
  vec3 col = 0.5 + 0.5 * cos(uTime + uv.xyx + vec3(0, 2, 4));
  gl_FragColor = vec4(col, 1.0);
}`
      },
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '400px',
        height: '320px'
      }
    })
  }
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

// Pass Vue Flow instance to the composable
const vueFlowInstance = useVueFlow()

// Use the composable to manage state and functionality
const {
  // State refs
  isHovered,
  shaderCanvas,
  fragmentShader,
  customStyle,
  
  // Computed properties
  resizeHandleStyle,
  
  // Methods
  onResize,
  run
} = useGLSLNode(props, emit, vueFlowInstance)
</script>

<style scoped>
@import '@/assets/css/nodes.css';

.glsl-canvas-container {
  flex: 1;
  width: 100%;
  position: relative;
  overflow: hidden;
  border-radius: 4px;
}

.shader-canvas {
  width: 100%;
  height: 100%;
  display: block;
}

.shader-textarea {
  height: 280px;
  font-family: monospace;
}

.run-shader-btn {
  background-color: #444;
  color: #eee;
  border: 1px solid #666;
  border-radius: 4px;
  padding: 6px 12px;
  margin-top: 10px;
  cursor: pointer;
  font-size: 14px;
  display: block;
  width: 100%;
}

.run-shader-btn:hover {
  background-color: #555;
}
</style>