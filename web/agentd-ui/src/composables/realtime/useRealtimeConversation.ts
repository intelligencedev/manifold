import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  type ComputedRef,
} from "vue";
import { transcribeRealtimeAudio } from "@/api/realtime";
import {
  AdaptiveVoiceActivityDetector,
  encodePCM16WAV,
  mergeAudioFrames,
  resampleLinear,
} from "@/lib/realtime/audio";
import { useChatStore } from "@/stores/chat";
import { useTtsStore } from "@/stores/tts";
import type { ChatInputRequest, ChatMessage } from "@/types/chat";

export type RealtimePhase =
  | "idle"
  | "connecting"
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

const TARGET_SAMPLE_RATE = 16_000;
const PRE_ROLL_FRAMES = 25;
const MAX_UTTERANCE_FRAMES = 750;
const TRAILING_SILENCE_FRAMES_TO_DROP = 18;

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

  let mediaStream: MediaStream | null = null;
  let audioContext: AudioContext | null = null;
  let sourceNode: MediaStreamAudioSourceNode | null = null;
  let captureNode: AudioWorkletNode | null = null;
  let silentGain: GainNode | null = null;
  let inputSampleRate = 48_000;
  let preRoll: Float32Array[] = [];
  let utteranceFrames: Float32Array[] = [];
  let collectingUtterance = false;
  let transcriptionQueue = Promise.resolve();
  let ttsWasEnabled = false;
  let realtimeSessionId: string | null = null;
  let callGeneration = 0;

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

  const phase: ComputedRef<RealtimePhase> = computed(() => {
    if (error.value) return "error";
    if (connecting.value) return "connecting";
    if (!callActive.value) return "idle";
    if (muted.value) return "muted";
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
    if (phase.value === "user-speaking") {
      return "Your turn will be sent automatically when you pause.";
    }
    return "WebGPU is preferred automatically, with CPU fallback when required.";
  });

  onMounted(async () => {
    try {
      await chat.init();
    } catch (loadError) {
      error.value = messageForError(loadError, "Could not load conversations.");
    }
  });

  onBeforeUnmount(() => {
    void endCall({ cancelResponse: false });
  });

  async function startCall() {
    if (!supported || callActive.value || connecting.value) return;
    const generation = ++callGeneration;
    error.value = "";
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
      preRoll = [];
      utteranceFrames = [];
      collectingUtterance = false;
      userSpeaking.value = false;
      audioLevel.value = 0;
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
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
        latency: { ideal: 0.01 },
      },
    });
    const AudioContextClass = audioContextConstructor();
    if (!AudioContextClass) throw new Error("Web Audio is unavailable.");
    audioContext = new AudioContextClass({ latencyHint: "interactive" });
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
      },
    );
    silentGain = audioContext.createGain();
    silentGain.gain.value = 0;
    captureNode.port.onmessage = handleWorkletMessage;
    sourceNode.connect(captureNode);
    captureNode.connect(silentGain);
    silentGain.connect(audioContext.destination);
    await audioContext.resume();
  }

  function handleWorkletMessage(event: MessageEvent<unknown>) {
    if (!callActive.value || muted.value) return;
    if (!event.data || typeof event.data !== "object") return;
    const data = event.data as { type?: unknown; samples?: unknown };
    if (data.type !== "audio" || !(data.samples instanceof Float32Array)) {
      return;
    }
    processAudioFrame(data.samples);
  }

  function processAudioFrame(frame: Float32Array) {
    const update = detector.process(frame);
    audioLevel.value = Math.min(
      1,
      update.level / Math.max(update.threshold * 2, 0.025),
    );

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
      detector.reset();
      queueTranscription(completedFrames);
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
      captureNode?.disconnect();
      silentGain?.disconnect();
    } catch {
      // Nodes may already be disconnected during browser teardown.
    }
    for (const track of mediaStream?.getTracks() || []) track.stop();
    if (audioContext && audioContext.state !== "closed") {
      await audioContext.close().catch(() => undefined);
    }
    mediaStream = null;
    audioContext = null;
    sourceNode = null;
    captureNode = null;
    silentGain = null;
  }

  return {
    supported,
    callActive,
    connecting,
    muted,
    audioLevel,
    liveTranscript,
    error,
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
  };
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
