<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container webgl-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    <!-- Input and Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
        
    <!-- Container for our WebGL canvas -->
    <div class="canvas-container" ref="canvasContainer">
      <canvas ref="webglCanvas"></canvas>
    </div>
        <!-- Resizer Component -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
        :min-width="300" :min-height="300" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { onMounted, onUnmounted } from "vue";
import { Handle } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { useWebGLNode } from "../../composables/useWebGLNode";

// --- PROPS & DEFAULTS ---
const props = defineProps({
  id: {
    type: String,
    required: true,
    default: "WebGLNode_0",
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: "WebGLNode",
      labelStyle: { fontWeight: "normal" },
      hasInputs: true,
      hasOutputs: true,
      // Default shader JSON renders a full-screen quad using a fragment shader
      // that uses u_time for animation. When a ResponseNode is connected, its
      // JSON output will update these shaders.
      inputs: {
        shaderJson: JSON.stringify(
          {
            vertexShader: `
                attribute vec2 a_Position;
                void main() {
                  gl_Position = vec4(a_Position, 0.0, 1.0);
                }
              `,
            fragmentShader: `
                precision mediump float;
                uniform vec2 u_resolution;
                uniform float u_time;
                // Signed distance function for a box.
                float sdBox(vec2 p, vec2 b) {
                  vec2 d = abs(p) - b;
                  return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
                }
                void main() {
                  // Map fragment coordinates to clip space [-1, 1]
                  vec2 uv = gl_FragCoord.xy / u_resolution.xy * 2.0 - 1.0;
                  uv.x *= u_resolution.x / u_resolution.y;
                  float angle = u_time;
                  vec2 rotatedUV = vec2(
                    uv.x * cos(angle) - uv.y * sin(angle),
                    uv.x * sin(angle) + uv.y * cos(angle)
                  );
                  float d = sdBox(rotatedUV, vec2(0.3, 0.3));
                  float color = step(0.0, -d);
                  gl_FragColor = vec4(vec3(color), 1.0);
                }
              `,
          },
          null,
          2
        ),
      },
      outputs: {},
      style: {
        border: "1px solid #666",
        borderRadius: "4px",
        backgroundColor: "#222",
        color: "#eee",
        width: "300px",
        height: "300px",
      },
    }),
  },
});

const emit = defineEmits(["update:data", "resize"]);

// Use the WebGLNode composable
const {
  webglCanvas,
  canvasContainer,
  customStyle,
  isHovered,
  resizeHandleStyle,
  onResize,
  setup,
  cleanup
} = useWebGLNode(props, emit);

// --- LIFECYCLE HOOKS ---
onMounted(() => {
  setup();
});

onUnmounted(() => {
  cleanup();
});
</script>

<style scoped>
.webgl-node {
  background-color: #222;
  border: 1px solid #666;
  border-radius: 4px;
  color: #eee;
  display: flex;
  flex-direction: column;
  position: relative;
}

.node-label {
  text-align: center;
  font-size: 16px;
  margin-bottom: 5px;
  padding: 5px;
}

.canvas-container {
  flex-grow: 1;
  position: relative;
  overflow: hidden;
}

canvas {
  width: 100%;
  height: 100%;
}
</style>