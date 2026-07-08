import { nextTick, ref, type Ref } from "vue";

export function useChatVoiceRecording({
  draft,
  autoSizeComposer,
}: {
  draft: Ref<string>;
  autoSizeComposer: () => void;
}) {
  const isRecording = ref(false);
  const canUseMic =
    typeof window !== "undefined" &&
    !!navigator.mediaDevices &&
    !!window.AudioContext;

  let mediaStream: MediaStream | null = null;
  let audioCtx: AudioContext | null = null;
  let processor: ScriptProcessorNode | null = null;
  let sourceNode: MediaStreamAudioSourceNode | null = null;
  let recordedChunks: Float32Array[] = [];
  let inputChannels = 1;
  let inputSampleRate = 48000;

  async function startRecording() {
    if (!canUseMic || isRecording.value) return;
    try {
      mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      audioCtx = new (
        window.AudioContext ||
        (window as Window & typeof globalThis & {
          webkitAudioContext?: typeof AudioContext;
        }).webkitAudioContext
      )();
      inputSampleRate = audioCtx.sampleRate || 48000;
      sourceNode = audioCtx.createMediaStreamSource(mediaStream);
      processor = audioCtx.createScriptProcessor(4096, 2, 1);
      inputChannels = sourceNode.channelCount || 1;
      recordedChunks = [];
      processor.onaudioprocess = (event: AudioProcessingEvent) => {
        const input0 = event.inputBuffer.getChannelData(0);
        let chunk: Float32Array;
        if (inputChannels > 1) {
          const input1 = event.inputBuffer.getChannelData(1);
          const mono = new Float32Array(input0.length);
          for (let i = 0; i < input0.length; i++) {
            mono[i] = (input0[i] + input1[i]) / 2;
          }
          chunk = mono;
        } else {
          chunk = new Float32Array(input0.length);
          chunk.set(input0);
        }
        recordedChunks.push(chunk);
      };
      sourceNode.connect(processor);
      processor.connect(audioCtx.destination);
      isRecording.value = true;
    } catch (error) {
      console.warn("Mic access failed", error);
      cleanupRecording();
    }
  }

  function cleanupRecording() {
    try {
      processor?.disconnect();
      sourceNode?.disconnect();
    } catch {}
    try {
      mediaStream?.getTracks().forEach((track) => track.stop());
    } catch {}
    try {
      audioCtx?.close();
    } catch {}
    mediaStream = null;
    processor = null;
    sourceNode = null;
    audioCtx = null;
  }

  async function stopRecording() {
    if (!isRecording.value) return;
    isRecording.value = false;
    cleanupRecording();
    const totalLen = recordedChunks.reduce((sum, chunk) => sum + chunk.length, 0);
    const merged = new Float32Array(totalLen);
    let offset = 0;
    for (const chunk of recordedChunks) {
      merged.set(chunk, offset);
      offset += chunk.length;
    }
    recordedChunks = [];
    const targetRate = 16000;
    const resampled = resampleLinear(merged, inputSampleRate, targetRate);
    const wavBlob = encodeWAV(resampled, targetRate);
    try {
      const text = await transcribeBlob(wavBlob);
      if (text) {
        const needsSpace = draft.value && !/\s$/.test(draft.value);
        draft.value = (draft.value || "") + (needsSpace ? " " : "") + text;
        nextTick(() => autoSizeComposer());
      }
    } catch (error) {
      console.warn("STT failed", error);
    }
  }

  return {
    isRecording,
    canUseMic,
    startRecording,
    stopRecording,
    cleanupRecording,
  };
}

function resampleLinear(
  input: Float32Array,
  inRate: number,
  outRate: number,
): Float32Array {
  if (inRate === outRate) return input;
  const ratio = inRate / outRate;
  const outLen = Math.floor(input.length / ratio);
  const out = new Float32Array(outLen);
  for (let i = 0; i < outLen; i++) {
    const idx = i * ratio;
    const i0 = Math.floor(idx);
    const i1 = Math.min(i0 + 1, input.length - 1);
    const frac = idx - i0;
    out[i] = input[i0] * (1 - frac) + input[i1] * frac;
  }
  return out;
}

function encodeWAV(samples: Float32Array, sampleRate: number): Blob {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  writeString(view, 0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeString(view, 8, "WAVE");
  writeString(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeString(view, 36, "data");
  view.setUint32(40, samples.length * 2, true);
  let offset = 44;
  for (let i = 0; i < samples.length; i++, offset += 2) {
    const sample = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(offset, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
  }
  return new Blob([view], { type: "audio/wav" });
}

function writeString(view: DataView, offset: number, value: string) {
  for (let i = 0; i < value.length; i++) {
    view.setUint8(offset + i, value.charCodeAt(i));
  }
}

async function transcribeBlob(blob: Blob): Promise<string> {
  const form = new FormData();
  form.set("audio", blob, "prompt.wav");
  const resp = await fetch("/stt", { method: "POST", body: form });
  if (!resp.ok) throw new Error(`stt failed (${resp.status})`);
  const data = (await resp.json()) as { text?: string };
  return data?.text || "";
}
