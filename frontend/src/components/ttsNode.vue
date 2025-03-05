<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">Text to Speech (WebGPU)</div>
        <Handle style="width:12px; height:12px" type="target" position="left" id="input" />
        <Handle style="width:12px; height:12px" type="source" position="right" id="output" />
        <div class="wave">
            <div class="bar" v-for="n in 10" :key="n" :ref="el => bars[n - 1] = el"></div>
        </div>
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :min-width="150" :min-height="100" :node-id="props.id" @resize="onResize" />
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
const resizeHandleStyle = computed(() => ({ visibility: isHovered.value ? "visible" : "hidden" }));
const bars = ref([]);

const audioQueue = ref([]);
const isProcessing = ref(false);  // Flag to indicate if TTS is currently processing
const isPlaying = ref(false);   // Flag to check if the current audio context is being played.

let audioContext = null;
let analyser = null;
let animationFrameId = null;
let tts = null;


async function initializeTTS() {
    try {
      tts = await KokoroTTS.from_pretrained("onnx-community/Kokoro-82M-v1.0-ONNX", { dtype: "fp32", device: "webgpu" });
    } catch(error) {
      console.error("Failed to initialize KokoroTTS:", error);
      // Handle initialization error appropriately.  Maybe set an error state.
    }
}

async function run() {
    try {
        const connectedSources = getEdges.value.filter((edge) => edge.target === props.id).map((edge) => edge.source);
        if (connectedSources.length > 0) {
            const sourceNode = findNode(connectedSources[0]);
            inputText.value = sourceNode.data.outputs.result.output;
        }

        const sentences = splitTextIntoChunks(inputText.value);
        audioQueue.value = []; // Clear the queue. Important for restarts.
        isProcessing.value = true;  // We are starting processing
        await processAudioQueue(sentences);
        return { success: true };

    } catch (error) {
        console.error(error);
        return { error };
    }
}


function splitTextIntoChunks(text) {
    const sentences = text.split(/(?<=[.!?])\s+/); // Split by sentence-ending punctuation.
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
        if (!isProcessing.value) break; // Stop if processing has been interrupted
        try {
          const audio = await tts.generate(chunk, { voice: "bf_isabella" });
          const audioWav = audio.toWav();
          audioQueue.value.push(audioWav); // Add to the play queue

          if (!isPlaying.value && audioQueue.value.length > 0) {
              playNextAudioChunk(); // Start playing if not already playing
          }
        } catch (error) {
            console.error("Error generating audio chunk:", error);
            // Consider:  Implement more robust error handling here, maybe retry or skip the chunk
            break; // Stop processing on error, or handle differently
        }
    }
    isProcessing.value = false; // Mark processing as complete (even if there was an error)
}



async function playNextAudioChunk() {
    if (audioQueue.value.length === 0) {
        isPlaying.value = false;
        return;
    }

    isPlaying.value = true;
    const audioWav = audioQueue.value.shift();  // Get the next audio chunk


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
        visualizeAudio(); // Only setup visualization once.
    } else {
      source.connect(analyser); //Connect it to the created analyser.
    }


    source.onended = () => {
      // Clean up if necessary
      if (audioQueue.value.length > 0) {
          playNextAudioChunk(); // Play the next chunk
      } else {
          isPlaying.value = false; // No more chunks to play
      }
    };
    source.start();
}


function visualizeAudio() {
    if (!analyser) return;  // Ensure analyser exists.

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

    // Only start a *new* animation if one isn't already running.
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
    isProcessing.value = false; // Ensure processing is stopped
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
.tts-node {
  background-color: #222;
  border: 1px solid #666;
  border-radius: 12px;
  color: #eee;
  display: flex;
  flex-direction: column;
  position: relative;
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