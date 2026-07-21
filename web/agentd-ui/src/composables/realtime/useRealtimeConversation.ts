import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  type ComputedRef,
} from "vue";
import rnnoiseWorkletPath from "@sapphi-red/web-noise-suppressor/rnnoiseWorklet.js?url";
import rnnoiseWasmPath from "@sapphi-red/web-noise-suppressor/rnnoise.wasm?url";
import rnnoiseWasmSimdPath from "@sapphi-red/web-noise-suppressor/rnnoise_simd.wasm?url";
import { transcribeRealtimeAudio } from "@/api/realtime";
import {
  AdaptiveVoiceActivityDetector,
  encodePCM16WAV,
  mergeAudioFrames,
  resampleLinear,
} from "@/lib/realtime/audio";
import {
  buildRealtimeAudioConstraints,
  detectRealtimeCaptureCapabilities,
  loadRealtimeAudioSettings,
  readAppliedCaptureSettings,
  saveRealtimeAudioSettings,
  shouldUseRnnoise,
  type AppliedRealtimeCaptureSettings,
  type NoiseSuppressionMode,
} from "@/lib/realtime/settings";
import { useChatStore } from "@/stores/chat";
import { useTtsStore } from "@/stores/tts";
import type { ChatInputRequest, ChatMessage } from "@/types/chat";

export type RealtimePhase =
  | "idle"
  | "connecting"
  | "calibrating"
  | "listening"
  | "muted"
  | "user-speaking"
  | "transcribing"
  | "thinking"
  | "assistant-speaking"
  | "error";

type PendingInputRequest = {
  messageId: string;
  request: ChatInputRequest;
};

export type RealtimeDenoiserStatus = "off" | "loading" | "active" | "fallback";

export type RealtimeAudioMetrics = {
  framesProcessed: number;
  rejectedNoiseEvents: number;
  deadlineMisses: number;
  noiseFloor: number;
  snrDb: number;
  speechProbability: number;
  processingMs: number;
  inputChannels: number;
  beamforming: boolean;
};

const TARGET_SAMPLE_RATE = 16_000;
const PRE_ROLL_FRAMES = 25;
const MAX_UTTERANCE_FRAMES = 750;
const TRAILING_SILENCE_FRAMES_TO_DROP = 18;
const INITIAL_CALIBRATION_FRAMES = 15;
const MANUAL_CALIBRATION_FRAMES = 75;
const WORKLET_DEADLINE_MS = 3;

export function useRealtimeConversation() {
  const chat = useChatStore();
  const tts = useTtsStore();
  const detector = new AdaptiveVoiceActivityDetector();

  const callActive = ref(false);
  const connecting = ref(false);
  const muted = ref(false);
  const userSpeaking = ref(false);
  const transcriptionsInFlight = ref(0);
  const audioLevel = ref(0);
  const liveTranscript = ref("");
  const error = ref("");
  const audioSettings = ref(loadRealtimeAudioSettings());
  const audioInputs = ref<MediaDeviceInfo[]>([]);
  const captureCapabilities = ref(detectRealtimeCaptureCapabilities());
  const appliedCaptureSettings = ref<AppliedRealtimeCaptureSettings | null>(
    null,
  );
  const denoiserStatus = ref<RealtimeDenoiserStatus>("off");
  const denoiserMessage = ref("");
  const calibrating = ref(false);
  const calibrationProgress = ref(0);
  const audioMetrics = ref<RealtimeAudioMetrics>(emptyAudioMetrics());

  let mediaStream: MediaStream | null = null;
  let audioContext: AudioContext | null = null;
  let sourceNode: MediaStreamAudioSourceNode | null = null;
  let captureNode: AudioWorkletNode | null = null;
  let rnnoiseNode: RealtimeRnnoiseNode | null = null;
  let silentGain: GainNode | null = null;
  let inputSampleRate = 48_000;
  let preRoll: Float32Array[] = [];
  let utteranceFrames: Float32Array[] = [];
  let collectingUtterance = false;
  let transcriptionQueue = Promise.resolve();
  let ttsWasEnabled = false;
  let realtimeSessionId: string | null = null;
  let callGeneration = 0;
  let calibrationTotalFrames = 0;
  let previousRejectedNoise = false;
  let consecutiveDeadlineMisses = 0;

  const activeSessionId = computed(() => chat.activeSessionId);
  const activeSession = computed(() => chat.activeSession);
  const sessions = computed(() => chat.sessions);
  const messages = computed(() => chat.activeMessages);
  const recentMessages = computed(() =>
    messages.value
      .filter(
        (message) => message.role === "user" || message.role === "assistant",
      )
      .slice(-6),
  );
  const latestAssistant = computed(() =>
    findLatestMessage(messages.value, "assistant"),
  );
  const assistantSpeaking = computed(() =>
    tts.isSpeakingForSession(activeSessionId.value),
  );
  const supported =
    typeof window !== "undefined" &&
    Boolean(navigator.mediaDevices?.getUserMedia) &&
    Boolean(audioContextConstructor());
  const selectedInputLabel = computed(() => {
    const selected = audioInputs.value.find(
      (device) => device.deviceId === audioSettings.value.inputDeviceId,
    );
    return selected?.label || "System microphone";
  });
  const captureCapabilityWarning = computed(() => {
    if (denoiserMessage.value) return denoiserMessage.value;
    if (!callActive.value || !appliedCaptureSettings.value) return "";
    const applied = appliedCaptureSettings.value;
    if (
      audioSettings.value.suppressionMode === "automatic" &&
      captureCapabilities.value.voiceIsolation &&
      applied.voiceIsolation !== true
    ) {
      return "The browser did not apply voice isolation; RNNoise fallback is active.";
    }
    if (
      audioSettings.value.suppressionMode === "standard" &&
      applied.noiseSuppression !== true
    ) {
      return "The browser did not apply its requested noise suppression.";
    }
    return "";
  });

  const phase: ComputedRef<RealtimePhase> = computed(() => {
    if (error.value) return "error";
    if (connecting.value) return "connecting";
    if (!callActive.value) return "idle";
    if (muted.value) return "muted";
    if (calibrating.value) return "calibrating";
    if (userSpeaking.value) return "user-speaking";
    if (transcriptionsInFlight.value > 0) return "transcribing";
    if (assistantSpeaking.value) return "assistant-speaking";
    if (chat.isStreaming) return "thinking";
    return "listening";
  });

  const statusLabel = computed(() => {
    switch (phase.value) {
      case "connecting":
        return "Preparing local voice models";
      case "listening":
        return "Listening";
      case "calibrating":
        return "Learning room noise";
      case "muted":
        return "Microphone muted";
      case "user-speaking":
        return "Listening to you";
      case "transcribing":
        return "Understanding";
      case "thinking":
        return latestAssistant.value?.content
          ? "Preparing speech"
          : "Manifold is thinking";
      case "assistant-speaking":
        return "Manifold is speaking";
      case "error":
        return "Realtime unavailable";
      default:
        return "Ready to start";
    }
  });

  const statusDetail = computed(() => {
    if (error.value) return error.value;
    if (!supported) {
      return "This browser does not expose the microphone APIs required for realtime mode.";
    }
    if (!callActive.value) {
      return "A private, local voice session using Moonshine STT and Supertonic TTS.";
    }
    if (phase.value === "listening") {
      return "Speak naturally. You can interrupt Manifold at any time.";
    }
    if (phase.value === "calibrating") {
      return "Stay quiet briefly while Manifold measures the ambient sound.";
    }
    if (phase.value === "user-speaking") {
      return "Your turn will be sent automatically when you pause.";
    }
    return "WebGPU is preferred automatically, with CPU fallback when required.";
  });

  onMounted(async () => {
    navigator.mediaDevices?.addEventListener?.(
      "devicechange",
      refreshAudioInputs,
    );
    await refreshAudioInputs();
    try {
      await chat.init();
    } catch (loadError) {
      error.value = messageForError(loadError, "Could not load conversations.");
    }
  });

  onBeforeUnmount(() => {
    navigator.mediaDevices?.removeEventListener?.(
      "devicechange",
      refreshAudioInputs,
    );
    void endCall({ cancelResponse: false });
  });

  async function startCall() {
    if (!supported || callActive.value || connecting.value) return;
    const generation = ++callGeneration;
    error.value = "";
    liveTranscript.value = "";
    audioMetrics.value = emptyAudioMetrics();
    connecting.value = true;
    try {
      // Create/resume the playback context while this is still a direct user
      // gesture so the first spoken response is never blocked by autoplay.
      await tts.unlockAudio();
      if (generation !== callGeneration) return;
      if (!activeSessionId.value) {
        await chat.createSession("Realtime call");
      }
      if (generation !== callGeneration) return;
      const sessionId = activeSessionId.value;
      if (!sessionId) throw new Error("No conversation is available.");

      ttsWasEnabled = tts.isEnabledForSession(sessionId);
      realtimeSessionId = sessionId;
      tts.setRealtimeSession(sessionId, true);
      // Model warmup runs beside microphone setup. Listening should not wait
      // for TTS initialization, and the streamer will await readiness itself.
      void tts.setSessionEnabled(sessionId, true);
      await openMicrophone();
      if (generation !== callGeneration) {
        await cleanupAudio();
        return;
      }
      callActive.value = true;
      beginCalibration(INITIAL_CALIBRATION_FRAMES);
    } catch (startError) {
      await cleanupAudio();
      const sessionId = realtimeSessionId;
      realtimeSessionId = null;
      if (sessionId) {
        tts.setRealtimeSession(sessionId, false);
        if (!ttsWasEnabled) await tts.setSessionEnabled(sessionId, false);
      }
      if (generation === callGeneration) {
        error.value = messageForError(
          startError,
          "Could not start the realtime conversation.",
        );
      }
    } finally {
      if (generation === callGeneration) connecting.value = false;
    }
  }

  async function endCall(
    options: { cancelResponse?: boolean } = { cancelResponse: true },
  ) {
    callGeneration += 1;
    connecting.value = false;
    const sessionId = realtimeSessionId;
    realtimeSessionId = null;
    callActive.value = false;
    muted.value = false;
    userSpeaking.value = false;
    audioLevel.value = 0;
    calibrating.value = false;
    calibrationProgress.value = 0;
    detector.reset();
    preRoll = [];
    utteranceFrames = [];
    collectingUtterance = false;
    await cleanupAudio();

    if (sessionId) {
      if (
        options.cancelResponse !== false &&
        chat.isSessionStreaming(sessionId)
      ) {
        chat.stopStreaming(sessionId, {
          reason: "Realtime call ended",
          markAsError: false,
        });
      }
      tts.stopSession(sessionId);
      tts.setRealtimeSession(sessionId, false);
      if (!ttsWasEnabled) await tts.setSessionEnabled(sessionId, false);
    }
  }

  function toggleMuted() {
    if (!callActive.value) return;
    muted.value = !muted.value;
    for (const track of mediaStream?.getAudioTracks() || []) {
      track.enabled = !muted.value;
    }
    if (muted.value) {
      detector.reset();
      calibrating.value = false;
      calibrationProgress.value = 0;
      preRoll = [];
      utteranceFrames = [];
      collectingUtterance = false;
      userSpeaking.value = false;
      audioLevel.value = 0;
    } else {
      beginCalibration(INITIAL_CALIBRATION_FRAMES);
    }
  }

  async function createNewConversation() {
    if (callActive.value) await endCall();
    error.value = "";
    await chat.createSession("Realtime call");
  }

  function selectConversation(sessionId: string) {
    if (callActive.value || !sessionId) return;
    error.value = "";
    chat.selectSession(sessionId);
  }

  function setInputDevice(deviceId: string) {
    if (callActive.value || connecting.value) return;
    audioSettings.value = { ...audioSettings.value, inputDeviceId: deviceId };
    persistAudioSettings();
  }

  function setSuppressionMode(mode: NoiseSuppressionMode) {
    if (callActive.value || connecting.value) return;
    audioSettings.value = { ...audioSettings.value, suppressionMode: mode };
    persistAudioSettings();
  }

  function setAutoGainControl(enabled: boolean) {
    if (callActive.value || connecting.value) return;
    audioSettings.value = { ...audioSettings.value, autoGainControl: enabled };
    persistAudioSettings();
  }

  function calibrateRoomNoise() {
    if (!callActive.value || phase.value !== "listening") return;
    preRoll = [];
    utteranceFrames = [];
    collectingUtterance = false;
    userSpeaking.value = false;
    beginCalibration(MANUAL_CALIBRATION_FRAMES);
  }

  function beginCalibration(frames: number) {
    calibrationTotalFrames = frames;
    calibrating.value = true;
    calibrationProgress.value = 0;
    detector.calibrate(frames);
    captureNode?.port.postMessage({ type: "calibrate", frames });
  }

  function persistAudioSettings() {
    saveRealtimeAudioSettings(audioSettings.value);
  }

  async function refreshAudioInputs() {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const devices = await navigator.mediaDevices.enumerateDevices();
      audioInputs.value = devices.filter(
        (device) => device.kind === "audioinput",
      );
      if (
        audioSettings.value.inputDeviceId &&
        !audioInputs.value.some(
          (device) => device.deviceId === audioSettings.value.inputDeviceId,
        )
      ) {
        audioSettings.value = { ...audioSettings.value, inputDeviceId: "" };
        persistAudioSettings();
      }
    } catch {
      audioInputs.value = [];
    }
  }

  function interruptAssistant() {
    const sessionId = activeSessionId.value;
    if (!sessionId) return;
    tts.stopSession(sessionId);
    // A pending input request is already waiting for the user's answer. Keep
    // that run alive so the transcript can be submitted to the request.
    if (
      chat.isSessionStreaming(sessionId) &&
      !latestPendingInputRequest(messages.value)
    ) {
      chat.stopStreaming(sessionId, {
        reason: "Interrupted in realtime",
        markAsError: false,
      });
    }
  }

  async function openMicrophone() {
    mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: buildRealtimeAudioConstraints(
        audioSettings.value,
        captureCapabilities.value,
      ),
    });
    const inputTrack = mediaStream.getAudioTracks()[0];
    if (!inputTrack)
      throw new Error("The selected microphone has no audio track.");
    appliedCaptureSettings.value = readAppliedCaptureSettings(inputTrack);
    await refreshAudioInputs();
    const AudioContextClass = audioContextConstructor();
    if (!AudioContextClass) throw new Error("Web Audio is unavailable.");
    audioContext = createRealtimeAudioContext(AudioContextClass);
    inputSampleRate = audioContext.sampleRate || 48_000;
    await audioContext.audioWorklet.addModule("/realtime-capture-worklet.js");
    sourceNode = audioContext.createMediaStreamSource(mediaStream);
    captureNode = new AudioWorkletNode(
      audioContext,
      "manifold-realtime-capture",
      {
        numberOfInputs: 1,
        numberOfOutputs: 1,
        outputChannelCount: [1],
        channelCount: Math.min(
          2,
          Math.max(1, appliedCaptureSettings.value?.channelCount || 1),
        ),
        channelCountMode: "max",
      },
    );
    silentGain = audioContext.createGain();
    silentGain.gain.value = 0;
    captureNode.port.onmessage = handleWorkletMessage;
    await connectCapturePipeline();
    captureNode.connect(silentGain);
    silentGain.connect(audioContext.destination);
    await audioContext.resume();
  }

  async function connectCapturePipeline() {
    if (!audioContext || !sourceNode || !captureNode) return;
    const useRnnoise = shouldUseRnnoise(
      audioSettings.value,
      appliedCaptureSettings.value,
    );
    if (!useRnnoise) {
      denoiserStatus.value = "off";
      denoiserMessage.value = "";
      sourceNode.connect(captureNode);
      return;
    }
    if (audioContext.sampleRate !== 48_000) {
      denoiserStatus.value = "fallback";
      denoiserMessage.value =
        "RNNoise requires 48 kHz capture; direct capture fallback is active.";
      sourceNode.connect(captureNode);
      return;
    }

    denoiserStatus.value = "loading";
    try {
      const { loadRnnoise, RnnoiseWorkletNode } =
        await import("@sapphi-red/web-noise-suppressor");
      const wasmBinary = await loadRnnoise({
        url: rnnoiseWasmPath,
        simdUrl: rnnoiseWasmSimdPath,
      });
      await audioContext.audioWorklet.addModule(rnnoiseWorkletPath);
      rnnoiseNode = new RnnoiseWorkletNode(audioContext, {
        wasmBinary,
        maxChannels: Math.min(
          2,
          Math.max(1, appliedCaptureSettings.value?.channelCount || 1),
        ),
      });
      rnnoiseNode.onprocessorerror = () => {
        fallbackFromRnnoise(
          "RNNoise stopped unexpectedly; direct capture fallback is active.",
        );
      };
      sourceNode.connect(rnnoiseNode);
      rnnoiseNode.connect(captureNode);
      denoiserStatus.value = "active";
      denoiserMessage.value = "";
    } catch (denoiserError) {
      console.warn("RNNoise initialization failed", denoiserError);
      const message =
        "RNNoise could not initialize; direct capture fallback is active.";
      if (rnnoiseNode) {
        fallbackFromRnnoise(message);
      } else {
        denoiserStatus.value = "fallback";
        denoiserMessage.value = message;
        sourceNode.connect(captureNode);
      }
    }
  }

  function fallbackFromRnnoise(message: string) {
    if (!sourceNode || !captureNode || !rnnoiseNode) return;
    const failedNode = rnnoiseNode;
    rnnoiseNode = null;
    try {
      sourceNode.disconnect(failedNode);
    } catch (fallbackError) {
      console.warn("RNNoise source disconnection failed", fallbackError);
    }
    try {
      failedNode.disconnect();
      failedNode.destroy();
    } catch (fallbackError) {
      console.warn("RNNoise cleanup failed", fallbackError);
    }
    try {
      sourceNode.connect(captureNode);
    } catch (fallbackError) {
      console.warn("RNNoise fallback connection failed", fallbackError);
    }
    denoiserStatus.value = "fallback";
    denoiserMessage.value = message;
  }

  function handleWorkletMessage(event: MessageEvent<unknown>) {
    if (!callActive.value || muted.value) return;
    if (!event.data || typeof event.data !== "object") return;
    const data = event.data as RealtimeWorkletAudioMessage;
    if (data.type !== "audio" || !(data.samples instanceof Float32Array)) {
      return;
    }
    processAudioFrame(data.samples, data);
  }

  function processAudioFrame(
    frame: Float32Array,
    metrics: RealtimeWorkletAudioMessage,
  ) {
    const update = detector.process(frame, {
      speechProbability: finiteNumber(metrics.speechProbability),
      noiseFloor: finiteNumber(metrics.noiseFloor),
      snrDb: finiteNumber(metrics.snrDb),
    });
    updateAudioMetrics(metrics, update.rejectedNoise);
    audioLevel.value = Math.min(
      1,
      update.level / Math.max(update.threshold * 2, 0.025),
    );
    if (calibrating.value) {
      const remaining = detector.calibrationRemaining;
      calibrationProgress.value = Math.min(
        1,
        1 - remaining / Math.max(1, calibrationTotalFrames),
      );
      if (remaining === 0) calibrating.value = false;
      return;
    }

    if (!collectingUtterance) {
      preRoll.push(frame);
      if (preRoll.length > PRE_ROLL_FRAMES) preRoll.shift();
    } else {
      utteranceFrames.push(frame);
    }

    if (update.event === "speech-start") {
      error.value = "";
      collectingUtterance = true;
      utteranceFrames = [...preRoll];
      preRoll = [];
      userSpeaking.value = true;
      liveTranscript.value = "";
      interruptAssistant();
      return;
    }

    if (
      collectingUtterance &&
      (update.event === "speech-end" ||
        utteranceFrames.length >= MAX_UTTERANCE_FRAMES)
    ) {
      const completedFrames = utteranceFrames;
      utteranceFrames = [];
      collectingUtterance = false;
      userSpeaking.value = false;
      detector.resetSpeechState();
      queueTranscription(completedFrames);
    }
  }

  function updateAudioMetrics(
    metrics: RealtimeWorkletAudioMessage,
    rejectedNoise: boolean,
  ) {
    const current = audioMetrics.value;
    const processingMs = finiteNumber(metrics.processingMs) || 0;
    const deadlineMiss = processingMs > WORKLET_DEADLINE_MS;
    consecutiveDeadlineMisses = deadlineMiss
      ? consecutiveDeadlineMisses + 1
      : 0;
    const rejectedEvent = rejectedNoise && !previousRejectedNoise;
    previousRejectedNoise = rejectedNoise;
    audioMetrics.value = {
      framesProcessed: current.framesProcessed + 1,
      rejectedNoiseEvents:
        current.rejectedNoiseEvents + (rejectedEvent ? 1 : 0),
      deadlineMisses: current.deadlineMisses + (deadlineMiss ? 1 : 0),
      noiseFloor: finiteNumber(metrics.noiseFloor) || current.noiseFloor,
      snrDb: finiteNumber(metrics.snrDb) || 0,
      speechProbability: finiteNumber(metrics.speechProbability) || 0,
      processingMs,
      inputChannels: finiteNumber(metrics.channels) || 1,
      beamforming: metrics.beamformed === true,
    };
    if (consecutiveDeadlineMisses >= 5 && rnnoiseNode) {
      consecutiveDeadlineMisses = 0;
      fallbackFromRnnoise(
        "Realtime audio repeatedly missed its processing deadline; RNNoise was bypassed.",
      );
    }
  }

  function queueTranscription(frames: Float32Array[]) {
    const trimmed =
      frames.length > TRAILING_SILENCE_FRAMES_TO_DROP
        ? frames.slice(0, -TRAILING_SILENCE_FRAMES_TO_DROP)
        : frames;
    transcriptionQueue = transcriptionQueue
      .then(() => transcribeAndDispatch(trimmed))
      .catch((transcriptionError) => {
        error.value = messageForError(
          transcriptionError,
          "Moonshine could not transcribe that turn.",
        );
      });
  }

  async function transcribeAndDispatch(frames: Float32Array[]) {
    if (!frames.length) return;
    transcriptionsInFlight.value += 1;
    try {
      const merged = mergeAudioFrames(frames);
      if (merged.length / inputSampleRate < 0.25) return;
      const audio = resampleLinear(merged, inputSampleRate, TARGET_SAMPLE_RATE);
      const text = await transcribeRealtimeAudio(
        encodePCM16WAV(audio, TARGET_SAMPLE_RATE),
      );
      if (!text || !callActive.value) return;
      liveTranscript.value = text;
      void dispatchTranscript(text).catch((dispatchError) => {
        error.value = messageForError(
          dispatchError,
          "Could not send the transcribed turn.",
        );
      });
    } finally {
      transcriptionsInFlight.value = Math.max(
        0,
        transcriptionsInFlight.value - 1,
      );
    }
  }

  async function dispatchTranscript(text: string) {
    const sessionId = activeSessionId.value;
    if (!sessionId || !callActive.value) return;
    const pending = latestPendingInputRequest(messages.value);
    if (pending) {
      await chat.submitInputRequest(
        sessionId,
        pending.messageId,
        pending.request.id,
        text,
      );
      return;
    }

    const session = activeSession.value;
    await chat.sendPrompt(text, [], undefined, {
      projectId: session?.projectId,
      memoryEnabled: session?.memoryEnabled,
      specialist: session?.activeTeam ? undefined : session?.activeSpecialist,
      teamName: session?.activeTeam,
    });
  }

  async function cleanupAudio() {
    if (captureNode) captureNode.port.onmessage = null;
    try {
      sourceNode?.disconnect();
      rnnoiseNode?.disconnect();
      captureNode?.disconnect();
      silentGain?.disconnect();
    } catch {
      // Nodes may already be disconnected during browser teardown.
    }
    try {
      rnnoiseNode?.destroy();
    } catch {
      // The worklet port may already be closed during browser teardown.
    }
    for (const track of mediaStream?.getTracks() || []) track.stop();
    if (audioContext && audioContext.state !== "closed") {
      await audioContext.close().catch(() => undefined);
    }
    mediaStream = null;
    audioContext = null;
    sourceNode = null;
    rnnoiseNode = null;
    captureNode = null;
    silentGain = null;
    appliedCaptureSettings.value = null;
    denoiserStatus.value = "off";
    denoiserMessage.value = "";
    consecutiveDeadlineMisses = 0;
    previousRejectedNoise = false;
  }

  return {
    supported,
    callActive,
    connecting,
    muted,
    audioLevel,
    liveTranscript,
    error,
    audioSettings,
    audioInputs,
    captureCapabilities,
    appliedCaptureSettings,
    selectedInputLabel,
    captureCapabilityWarning,
    denoiserStatus,
    denoiserMessage,
    calibrating,
    calibrationProgress,
    audioMetrics,
    phase,
    statusLabel,
    statusDetail,
    sessions,
    activeSession,
    activeSessionId,
    recentMessages,
    startCall,
    endCall,
    toggleMuted,
    interruptAssistant,
    createNewConversation,
    selectConversation,
    setInputDevice,
    setSuppressionMode,
    setAutoGainControl,
    calibrateRoomNoise,
    refreshAudioInputs,
  };
}

type RealtimeWorkletAudioMessage = {
  type?: unknown;
  samples?: unknown;
  sequence?: unknown;
  channels?: unknown;
  beamformed?: unknown;
  speechProbability?: unknown;
  noiseFloor?: unknown;
  snrDb?: unknown;
  rejectedNoise?: unknown;
  processingMs?: unknown;
};

type RealtimeRnnoiseNode = AudioWorkletNode & { destroy(): void };

function emptyAudioMetrics(): RealtimeAudioMetrics {
  return {
    framesProcessed: 0,
    rejectedNoiseEvents: 0,
    deadlineMisses: 0,
    noiseFloor: 0,
    snrDb: 0,
    speechProbability: 0,
    processingMs: 0,
    inputChannels: 1,
    beamforming: false,
  };
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function audioContextConstructor(): typeof AudioContext | undefined {
  if (typeof window === "undefined") return undefined;
  return (
    window.AudioContext ||
    (
      window as typeof window & {
        webkitAudioContext?: typeof AudioContext;
      }
    ).webkitAudioContext
  );
}

function createRealtimeAudioContext(
  Context: typeof AudioContext,
): AudioContext {
  try {
    return new Context({ latencyHint: "interactive", sampleRate: 48_000 });
  } catch {
    return new Context({ latencyHint: "interactive" });
  }
}

function findLatestMessage(
  messages: ChatMessage[],
  role: ChatMessage["role"],
): ChatMessage | undefined {
  return [...messages].reverse().find((message) => message.role === role);
}

function latestPendingInputRequest(
  messages: ChatMessage[],
): PendingInputRequest | null {
  for (
    let messageIndex = messages.length - 1;
    messageIndex >= 0;
    messageIndex -= 1
  ) {
    const message = messages[messageIndex];
    const request = [...(message.inputRequests || [])]
      .reverse()
      .find(
        (candidate) =>
          candidate.status === "pending" || candidate.status === "error",
      );
    if (request) return { messageId: message.id, request };
  }
  return null;
}

function messageForError(error: unknown, fallback: string): string {
  if (error instanceof DOMException && error.name === "NotAllowedError") {
    return "Microphone access was denied. Allow microphone access and try again.";
  }
  return error instanceof Error && error.message ? error.message : fallback;
}

export type RealtimeConversation = ReturnType<typeof useRealtimeConversation>;
