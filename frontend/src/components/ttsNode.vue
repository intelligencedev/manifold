<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">
      Text to Speech (WebGPU)
      <select v-model="selectedVoice" class="voice-select">
        <optgroup label="American (Female)">
          <option value="af_alloy">af_alloy</option>
          <option value="af_aoede">af_aoede</option>
          <option value="af_bella">af_bella</option>
          <option value="af_heart">af_heart</option>
          <option value="af_jessica">af_jessica</option>
          <option value="af_kore">af_kore</option>
          <option value="af_nicole">af_nicole</option>
          <option value="af_nova">af_nova</option>
          <option value="af_river">af_river</option>
          <option value="af_sarah">af_sarah</option>
          <option value="af_sky">af_sky</option>
        </optgroup>
        <optgroup label="American (Male)">
          <option value="am_adam">am_adam</option>
          <option value="am_echo">am_echo</option>
          <option value="am_eric">am_eric</option>
          <option value="am_fenrir">am_fenrir</option>
          <option value="am_liam">am_liam</option>
          <option value="am_michael">am_michael</option>
          <option value="am_onyx">am_onyx</option>
          <option value="am_puck">am_puck</option>
        </optgroup>
        <optgroup label="British (Female)">
          <option value="bf_alice">bf_alice</option>
          <option value="bf_emma">bf_emma</option>
          <option value="bf_isabella">bf_isabella</option>
          <option value="bf_lily">bf_lily</option>
        </optgroup>
        <optgroup label="British (Male)">
          <option value="bm_daniel">bm_daniel</option>
          <option value="bm_fable">bm_fable</option>
          <option value="bm_george">bm_george</option>
          <option value="bm_lewis">bm_lewis</option>
        </optgroup>
      </select>
    </div>
    <Handle style="width:12px; height:12px" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" type="source" position="right" id="output" />
    <div class="wave">
      <div class="bar" v-for="n in 10" :key="n" :ref="el => bars[n - 1] = el"></div>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle(isHovered)"
      :line-style="resizeHandleStyle(isHovered)"
      :min-width="150"
      :min-height="100"
      :node-id="props.id"
      @resize="(evt) => onResize(evt, emit)"
    />
  </div>
</template>

<script setup>
import { nextTick, onMounted, onUnmounted } from "vue";
import { Handle } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { useTtsNode } from "../composables/useTtsNode";

const props = defineProps({
  id: { type: String, required: true, default: "ttsNode_0" },
  data: { type: Object, required: false, default: () => ({}) },
});

const emit = defineEmits(["update:data", "resize"]);

// Use the composable
const {
  selectedVoice,
  bars,
  isHovered,
  customStyle,
  resizeHandleStyle,
  run,
  setupNode,
  cleanupNode,
  onResize
} = useTtsNode(props);

// Lifecycle hooks
onMounted(() => {
  setupNode();
});

onUnmounted(() => {
  cleanupNode();
});
</script>

<style scoped>
.node-container {
  background-color: #222;
  border: 1px solid #666;
  border-radius: 12px;
  color: #eee;
  display: flex;
  flex-direction: column;
  position: relative;
  padding: 8px;
}

.node-label {
  font-weight: bold;
  margin-bottom: 8px;
}

.voice-select {
  margin-top: 8px;
  padding: 4px;
  font-size: 14px;
  border: 1px solid #666;
  background-color: #222;
  color: #eee;
  border-radius: 4px;
  width: 100%;
}

.wave {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 60px;
  padding: 10px;
  margin-top: 12px;
}

.bar {
  width: 8px;
  height: 40px;
  background-color: #4afff0;
  border-radius: 4px;
  transform-origin: bottom;
  transition: transform 0.1s ease;
}
</style>
