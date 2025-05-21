<template>
  <BaseNode :id="id" :data="data" :min-height="100" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label font-bold mb-2">
        Text to Speech (WebGPU)
        <select v-model="selectedVoice" class="voice-select mt-2 p-1 text-sm border border-gray-600 bg-zinc-900 text-zinc-100 rounded w-full">
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
    </template>
    <Handle style="width:12px; height:12px" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" type="source" position="right" id="output" />
    <div class="wave flex items-center justify-between h-16 p-2 mt-3">
      <div class="bar w-2 h-10 bg-teal-400 rounded origin-bottom transition-transform" v-for="n in 10" :key="n" :ref="el => bars[n - 1] = el"></div>
    </div>
  </BaseNode>
</template>

<script setup>
import { onMounted, onUnmounted } from 'vue'
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import { useTtsNode } from '@/composables/useTtsNode'

const props = defineProps({
  id: { type: String, required: true, default: 'ttsNode_0' },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'ttsNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '352px',
        height: '240px'
      }
    })
  }
})

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
} = useTtsNode(props, emit);

// Lifecycle hooks
onMounted(() => {
  setupNode();
});

onUnmounted(() => {
  cleanupNode();
});
</script>

<style scoped>
/* Tailwind handles styling */
</style>
