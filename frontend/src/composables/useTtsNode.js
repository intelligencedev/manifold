import { ref, onMounted, onUnmounted } from 'vue';
import { useVueFlow } from '@vue-flow/core';
import { KokoroTTS } from 'kokoro-js';

export function useTtsNode(props) {
  const { getEdges, findNode } = useVueFlow();
  
  const inputText = ref('');
  const selectedVoice = ref('af_sky');
  const audioQueue = ref([]);
  const isProcessing = ref(false);
  const isPlaying = ref(false);
  const bars = ref([]);
  const isHovered = ref(false);
  const customStyle = ref({});
  
  let audioContext = null;
  let analyser = null;
  let animationFrameId = null;
  let tts = null;

  const resizeHandleStyle = (isHovered) => ({
    visibility: isHovered ? 'visible' : 'hidden',
  });

  async function initializeTTS() {
    try {
      tts = await KokoroTTS.from_pretrained(
        'onnx-community/Kokoro-82M-v1.0-ONNX',
        { dtype: 'fp32', device: 'webgpu' }
      );
    } catch (error) {
      console.error('Failed to initialize KokoroTTS:', error);
    }
  }

  function splitTextIntoChunks(text) {
    const sentences = text.split(/(?<=[.!?])\s+/);
    const chunks = [];
    for (let i = 0; i < sentences.length; i += 2) {
      chunks.push(sentences.slice(i, i + 2).join(' '));
    }
    return chunks;
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

  async function processAudioQueue(sentences) {
    if (!tts) {
      console.error('TTS not initialized.');
      return;
    }
    for (const chunk of sentences) {
      if (!isProcessing.value) break;
      try {
        // Use the selected voice from the dropdown
        const audio = await tts.generate(chunk, { voice: selectedVoice.value });
        const audioWav = audio.toWav();
        audioQueue.value.push(audioWav);

        if (!isPlaying.value && audioQueue.value.length > 0) {
          playNextAudioChunk();
        }
      } catch (error) {
        console.error('Error generating audio chunk:', error);
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
        if (bar) {
          bar.style.transform = `scaleY(${scaleY})`;
          bar.style.backgroundColor = `rgb(${dataArray[idx * 10]}, 74, 240)`;
        }
      });
      animationFrameId = requestAnimationFrame(animate);
    };

    if (!animationFrameId) {
      animate();
    }
  }

  const onResize = (event, emit) => {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    emit('resize', { id: props.id, width: event.width, height: event.height });
  };

  const setupNode = async () => {
    await initializeTTS();
    if (!props.data.run) props.data.run = run;
  };

  const cleanupNode = () => {
    if (audioContext) audioContext.close();
    if (animationFrameId) cancelAnimationFrame(animationFrameId);
    isProcessing.value = false;
    isPlaying.value = false;
  };

  return {
    selectedVoice,
    bars,
    isHovered,
    customStyle,
    resizeHandleStyle,
    run,
    setupNode,
    cleanupNode,
    onResize
  };
}