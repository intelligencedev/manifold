<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container webgl-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Input and Output Handles -->
    <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" position="right" id="output" />
        
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
import { ref, computed, onMounted, nextTick, onUnmounted, watch } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";

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

// --- VUE FLOW HELPERS ---
const { getEdges, findNode } = useVueFlow();

// --- REACTIVE REFERENCES ---
const webglCanvas = ref(null);
const canvasContainer = ref(null);
const customStyle = ref({});
const isHovered = ref(false);
const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? "visible" : "hidden",
}));

// --- SHADER JSON HANDLING ---
// Using a computed property so that we can update the node's data.
const shaderJson = computed({
  get: () => props.data.inputs?.shaderJson || "{}",
  set: (value) => {
    props.data.inputs.shaderJson = value;
    updateNodeData();
  },
});
function updateNodeData() {
  const updatedData = {
    ...props.data,
    inputs: { shaderJson: shaderJson.value },
    outputs: props.data.outputs,
  };
  emit("update:data", { id: props.id, data: updatedData });
}

// --- ANIMATION LOOP MANAGEMENT ---
let animationFrameId = null;

// --- RUN FUNCTION ---
// This function is called when the node is executed. It checks for a connected
// ResponseNode to update the shader JSON, then re-initializes WebGL.
async function run() {
  console.log("Running WebGLNode:", props.id);

  try {
    // Look for connected nodes (assumed to be a ResponseNode providing shader output).
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0]);
      const responseOutput = sourceNode.data.outputs?.result?.output;
      if (responseOutput) {
        try {
          const parsedShader = JSON.parse(responseOutput);
          if (parsedShader.vertexShader && parsedShader.fragmentShader) {
            shaderJson.value = responseOutput;
            console.log("Shader JSON updated from ResponseNode:", parsedShader);
          } else {
            console.error("The parsed shader JSON is missing required properties.");
          }
        } catch (parseError) {
          console.error("Error parsing shader JSON from ResponseNode:", parseError);
        }
      }
    }

    await nextTick();
    initializeWebGL();
    return { success: true };
  } catch (error) {
    console.error("Error in run():", error);
    props.data.outputs.result = { error: error.message };
    return { error };
  }
}

// --- WATCHER ---
// When the shader JSON changes, reinitialize WebGL immediately.
watch(
  () => props.data.inputs.shaderJson,
  (newVal, oldVal) => {
    console.log("Shader JSON changed, reinitializing WebGL.");
    nextTick(() => {
      initializeWebGL();
    });
  }
);

// --- INITIALIZE WEBGL ---
// Sets up shaders, geometry, and an animation loop to update u_time.
// Uses a full-screen quad that covers the entire canvas.
function initializeWebGL() {
  const canvas = webglCanvas.value;
  if (!canvas || !canvasContainer.value) {
    console.error("Canvas or container not found");
    return;
  }

  // Update canvas dimensions to match the container.
  const width = canvasContainer.value.clientWidth;
  const height = canvasContainer.value.clientHeight;
  canvas.width = width;
  canvas.height = height;

  const gl = canvas.getContext("webgl") || canvas.getContext("experimental-webgl");
  if (!gl) {
    console.error("Unable to initialize WebGL. Your browser may not support it.");
    return;
  }

  // Parse the shader JSON.
  let shaderObj;
  try {
    shaderObj = JSON.parse(shaderJson.value);
  } catch (err) {
    console.error("Invalid JSON for shaders:", err);
    props.data.outputs.result = { error: "Invalid JSON for shaders" };
    return;
  }

  const vShaderSource = shaderObj.vertexShader;
  const fShaderSource = shaderObj.fragmentShader;

  if (!vShaderSource || !fShaderSource) {
    console.error("Missing shader code. Both vertexShader and fragmentShader are required.");
    props.data.outputs.result = {
      error: "Missing shader code. Both vertexShader and fragmentShader are required.",
    };
    return;
  }

  // Compile shaders.
  const vShader = compileShader(gl, gl.VERTEX_SHADER, vShaderSource);
  const fShader = compileShader(gl, gl.FRAGMENT_SHADER, fShaderSource);
  if (!vShader || !fShader) return;

  // Create and link the WebGL program.
  const program = createProgram(gl, vShader, fShader);
  if (!program) return;
  gl.useProgram(program);

  // Set uniforms.
  const uResolutionLocation = gl.getUniformLocation(program, "u_resolution");
  if (uResolutionLocation) {
    gl.uniform2f(uResolutionLocation, canvas.width, canvas.height);
  }
  const uTimeLocation = gl.getUniformLocation(program, "u_time");

  // Set viewport and clear color.
  gl.viewport(0, 0, canvas.width, canvas.height);
  gl.clearColor(0.0, 0.0, 0.0, 1.0);

  // Define a full-screen quad (using clip-space coordinates).
  const vertices = new Float32Array([
    -1.0, 1.0,  // Top-left
    -1.0, -1.0,  // Bottom-left
    1.0, 1.0,  // Top-right
    1.0, -1.0,  // Bottom-right
  ]);
  const vertexBuffer = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, vertexBuffer);
  gl.bufferData(gl.ARRAY_BUFFER, vertices, gl.STATIC_DRAW);

  const a_Position = gl.getAttribLocation(program, "a_Position");
  if (a_Position < 0) {
    console.error("Failed to get the storage location of a_Position");
    return;
  }
  gl.vertexAttribPointer(a_Position, 2, gl.FLOAT, false, 0, 0);
  gl.enableVertexAttribArray(a_Position);

  // Cancel any existing animation loop.
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId);
    animationFrameId = null;
  }

  const startTime = performance.now();
  function animate() {
    const currentTime = performance.now();
    const elapsed = (currentTime - startTime) / 1000.0; // seconds
    if (uTimeLocation) {
      gl.uniform1f(uTimeLocation, elapsed);
    }
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    animationFrameId = requestAnimationFrame(animate);
  }
  animate();
}

// --- SHADER UTILS ---
function compileShader(gl, type, source) {
  const shader = gl.createShader(type);
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  const success = gl.getShaderParameter(shader, gl.COMPILE_STATUS);
  if (!success) {
    console.error("Could not compile shader:", gl.getShaderInfoLog(shader));
    gl.deleteShader(shader);
    return null;
  }
  return shader;
}

function createProgram(gl, vertexShader, fragmentShader) {
  const program = gl.createProgram();
  gl.attachShader(program, vertexShader);
  gl.attachShader(program, fragmentShader);
  gl.linkProgram(program);
  const success = gl.getProgramParameter(program, gl.LINK_STATUS);
  if (!success) {
    console.error("Program failed to link:", gl.getProgramInfoLog(program));
    gl.deleteProgram(program);
    return null;
  }
  return program;
}

// --- RESIZE HANDLER ---
const onResize = (event) => {
  customStyle.value.width = `${event.width}px`;
  customStyle.value.height = `${event.height}px`;
  nextTick(() => {
    initializeWebGL();
  });
  emit("resize", { id: props.id, width: event.width, height: event.height });
};

// --- LIFECYCLE HOOKS ---
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run;
  }
  initializeWebGL();
});

onUnmounted(() => {
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId);
  }
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