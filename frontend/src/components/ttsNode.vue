<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container tts-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">Text to Speech (WebGPU)</div>
        <Handle style="width:12px; height:12px" type="target" position="left" id="input" />
        <Handle style="width:12px; height:12px" type="source" position="right" id="output" />
        <div class="text-display" v-if="inputText">
            Input Text: {{ inputText }}
        </div>
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :min-width="150" :min-height="100" :node-id="props.id" @resize="onResize" />
    </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick, onUnmounted } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { KokoroTTS, TextSplitterStream } from "kokoro-js";

const props = defineProps({
    id: { type: String, required: true, default: "ttsNode_0" },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: "ttsNode",
            labelStyle: { fontWeight: "normal" },
            hasInputs: true,
            hasOutputs: true,
            inputs: { text: "" },
            outputs: { text: "", audioStreamId: null },
            style: {
                border: "1px solid #666",
                borderRadius: "4px",
                backgroundColor: "#222",
                color: "#eee",
                width: "200px",
                height: "150px",
            },
        }),
    },
});
const emit = defineEmits(["update:data", "resize"]);

const { getEdges, findNode } = useVueFlow();

const customStyle = ref({});
const isHovered = ref(false);
const inputText = ref("");
const audioContext = ref(null);

const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? "visible" : "hidden",
    width: "12px",
    height: "12px",
}));

let audioStreamIdCounter = 0;
let ttsModel = null;

async function initTTSModel() {
    if (!ttsModel) {
        console.log(`[ttsNode ${props.id}] Initializing Kokoro TTS model`);
        const model_id = "onnx-community/Kokoro-82M-v1.0-ONNX";
        ttsModel = await KokoroTTS.from_pretrained(model_id, { dtype: "fp32" });
        console.log(`[ttsNode ${props.id}] Kokoro TTS model initialized`);
    }
    return ttsModel;
}

async function run() {
    console.log(`[ttsNode ${props.id}] Starting execution`);
    var textInput = props.data.inputs.text;
    try {
        const connectedSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => edge.source);

        if (connectedSources.length > 0) {
            for (const sourceId of connectedSources) {
                const sourceNode = findNode(sourceId);
                if (sourceNode) {
                    textInput = `${sourceNode.data.outputs.result.output}`;
                }
            }
        }

        const model_id = "onnx-community/Kokoro-82M-v1.0-ONNX";
        const tts = await KokoroTTS.from_pretrained(model_id, {
            dtype: "fp32", // Options: "fp32", "fp16", "q8", "q4", "q4f16"
            device: "webgpu", // Options: "wasm", "webgpu" (web) or "cpu" (node). If using "webgpu", we recommend using dtype="fp32".
        });

        const audio = await tts.generate(textInput, {
            // Use `tts.list_voices()` to list all available voices
            voice: "af_heart",
        });

        var audioWav = audio.toWav();

        // Play the audio
        const audioContext = new AudioContext();
        const audioBuffer = await audioContext.decodeAudioData(audioWav);
        const audioSource = audioContext.createBufferSource();
        audioSource.buffer = audioBuffer;
        audioSource.connect(audioContext.destination);
        audioSource.start();

        props.data.outputs.text = inputText.value;


        console.log(`[ttsNode ${props.id}] Execution completed`);
        return { success: true };
    } catch (error) {
        console.error(`[ttsNode ${props.id}] Execution error:`, error);
        props.data.outputs.result = { error: error.message };
        updateNodeData();
        return { error };
    }
}

function updateNodeData() {
    emit("update:data", {
        id: props.id,
        data: { ...props.data, inputs: { ...props.data.inputs }, outputs: { ...props.data.outputs } },
    });
}

onMounted(() => {
    if (!props.data.run) props.data.run = run;
    initTTSModel().catch((err) => console.error(`[ttsNode ${props.id}] TTS preload error:`, err));
});

onUnmounted(() => {
    ttsModel = null;
    if (audioContext.value) {
        audioContext.value.close();
        audioContext.value = null;
    }
});

const onResize = (event) => {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    nextTick(() => { });
    emit("resize", { id: props.id, width: event.width, height: event.height });
};
</script>
