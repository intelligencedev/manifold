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

let audioContext = null;
let analyser = null;
let animationFrameId = null;

async function run() {
    try {
        const connectedSources = getEdges.value.filter((edge) => edge.target === props.id).map((edge) => edge.source);
        if (connectedSources.length > 0) {
            const sourceNode = findNode(connectedSources[0]);
            inputText.value = sourceNode.data.outputs.result.output;
        }

        const tts = await KokoroTTS.from_pretrained("onnx-community/Kokoro-82M-v1.0-ONNX", { dtype: "fp32", device: "webgpu" });
        const audio = await tts.generate(inputText.value, { voice: "bf_isabella" });
        const audioWav = audio.toWav();

        audioContext = new AudioContext();
        const audioBuffer = await audioContext.decodeAudioData(audioWav);
        const source = audioContext.createBufferSource();
        source.buffer = audioBuffer;

        analyser = audioContext.createAnalyser();
        source.connect(analyser);
        analyser.connect(audioContext.destination);

        source.start();
        visualizeAudio();

        return { success: true };
    } catch (error) {
        console.error(error);
        return { error };
    }
}

function visualizeAudio() {
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
    animate();
}

onMounted(() => {
    if (!props.data.run) props.data.run = run;
});

onUnmounted(() => {
    if (audioContext) audioContext.close();
    if (animationFrameId) cancelAnimationFrame(animationFrameId);
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
