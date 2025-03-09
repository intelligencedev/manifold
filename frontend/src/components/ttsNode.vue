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
        :handle-style="resizeHandleStyle"
        :line-style="resizeHandleStyle"
        :min-width="150"
        :min-height="100"
        :node-id="props.id"
        @resize="onResize"
      />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, onMounted, nextTick, onUnmounted } from "vue";
  import { Handle, useVueFlow } from "@vue-flow/core";
  import { NodeResizer } from "@vue-flow/node-resizer";
  import { KokoroTTS } from "kokoro-js";
  
  const props = defineProps({
    id: { type: String, required: true, default: "ttsNode_0" },
    data: { type: Object, required: false, default: () => ({}) },
  });
  
  const emit = defineEmits(["update:data", "resize"]);
  const { getEdges, findNode } = useVueFlow();
  const customStyle = ref({});
  const isHovered = ref(false);
  const inputText = ref("");
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? "visible" : "hidden",
  }));
  const bars = ref([]);
  
  // The voice selection (defaulting to "af_sky")
  const selectedVoice = ref("af_sky");
  
  const audioQueue = ref([]);
  const isProcessing = ref(false);
  const isPlaying = ref(false);
  
  let audioContext = null;
  let analyser = null;
  let animationFrameId = null;
  let tts = null;
  
  async function initializeTTS() {
    try {
      tts = await KokoroTTS.from_pretrained(
        "onnx-community/Kokoro-82M-v1.0-ONNX",
        { dtype: "fp32", device: "webgpu" }
      );
    } catch (error) {
      console.error("Failed to initialize KokoroTTS:", error);
      // Optionally, set an error state here
    }
  }
  
  async function run() {
    try {
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source);
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0]);
        inputText.value = sourceNode.data.outputs.result.output;
      }
  
      const sentences = splitTextIntoChunks(inputText.value);
      audioQueue.value = []; // Clear the queue in case of restarts
      isProcessing.value = true;
      await processAudioQueue(sentences);
      return { success: true };
    } catch (error) {
      console.error(error);
      return { error };
    }
  }
  
  function splitTextIntoChunks(text) {
    const sentences = text.split(/(?<=[.!?])\s+/);
    const chunks = [];
    for (let i = 0; i < sentences.length; i += 2) {
      chunks.push(sentences.slice(i, i + 2).join(" "));
    }
    return chunks;
  }
  
  async function processAudioQueue(sentences) {
    if (!tts) {
      console.error("TTS not initialized.");
      return;
    }
    for (const chunk of sentences) {
      if (!isProcessing.value) break;
      try {
        // Use the selected voice from the dropdown here
        const audio = await tts.generate(chunk, { voice: selectedVoice.value });
        const audioWav = audio.toWav();
        audioQueue.value.push(audioWav);
  
        if (!isPlaying.value && audioQueue.value.length > 0) {
          playNextAudioChunk();
        }
      } catch (error) {
        console.error("Error generating audio chunk:", error);
        break;
      }
    }
    isProcessing.value = false;
  }
  
  async function playNextAudioChunk() {
    if (audioQueue.value.length === 0) {
      isPlaying.value = false;
      return;
    }
  
    isPlaying.value = true;
    const audioWav = audioQueue.value.shift();
  
    if (!audioContext) {
      audioContext = new AudioContext();
    }
    const audioBuffer = await audioContext.decodeAudioData(audioWav);
    const source = audioContext.createBufferSource();
    source.buffer = audioBuffer;
  
    if (!analyser) {
      analyser = audioContext.createAnalyser();
      source.connect(analyser);
      analyser.connect(audioContext.destination);
      visualizeAudio();
    } else {
      source.connect(analyser);
    }
  
    source.onended = () => {
      if (audioQueue.value.length > 0) {
        playNextAudioChunk();
      } else {
        isPlaying.value = false;
      }
    };
    source.start();
  }
  
  function visualizeAudio() {
    if (!analyser) return;
  
    const dataArray = new Uint8Array(analyser.frequencyBinCount);
  
    const animate = () => {
      analyser.getByteFrequencyData(dataArray);
      bars.value.forEach((bar, idx) => {
        const scaleY = dataArray[idx * 10] / 128;
        bar.style.transform = `scaleY(${scaleY})`;
        bar.style.backgroundColor = `rgb(${dataArray[idx * 10]}, 74, 240)`;
      });
      animationFrameId = requestAnimationFrame(animate);
    };
  
    if (!animationFrameId) {
      animate();
    }
  }
  
  onMounted(async () => {
    await initializeTTS();
    if (!props.data.run) props.data.run = run;
  });
  
  onUnmounted(() => {
    if (audioContext) audioContext.close();
    if (animationFrameId) cancelAnimationFrame(animationFrameId);
    isProcessing.value = false;
    isPlaying.value = false;
  });
  
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    nextTick();
    emit("resize", { id: props.id, width: event.width, height: event.height });
  };
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
  