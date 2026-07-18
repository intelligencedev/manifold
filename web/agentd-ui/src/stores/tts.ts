import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  defaultSupertonicTtsSettings,
  isValidVoiceStylePayload,
  loadSupertonicTtsSettings,
  normalizeSettings,
  saveSupertonicTtsSettings,
  type CustomVoiceStyle,
  type SupertonicTtsSettings,
} from "@/lib/tts/supertonic/settings";
import {
  PRESET_VOICE_IDS,
  type EngineStatus,
  type SupertonicLoadProgress,
} from "@/lib/tts/supertonic/constants";
import { serverEngine as supertonicEngine } from "@/lib/tts/supertonic/serverEngine";
import { SupertonicStreamer } from "@/lib/tts/supertonic/streamer";

export const useTtsStore = defineStore("supertonic-tts", () => {
  const settings = ref<SupertonicTtsSettings>(loadSupertonicTtsSettings());
  const engineStatus = ref<EngineStatus>(supertonicEngine.getStatus());
  const engineError = ref("");
  const progress = ref<SupertonicLoadProgress | null>(null);
  const backend = ref<"webgpu" | "wasm" | "server" | null>(null);

  const streamer = new SupertonicStreamer({
    getSettings: () => settings.value,
    isEnabledForSession: (sessionId: string) => isEnabledForSession(sessionId),
  });

  if (typeof window !== "undefined") {
    supertonicEngine.onStatus((status) => {
      engineStatus.value = status;
      backend.value = supertonicEngine.getBackend();
      engineError.value = supertonicEngine.getError();
    });
    supertonicEngine.onProgress((p) => {
      progress.value = p;
    });
    supertonicEngine.setCustomVoices(settings.value.customVoices);
  }

  function persist() {
    saveSupertonicTtsSettings(settings.value);
  }

  function patchSettings(partial: Partial<SupertonicTtsSettings>) {
    settings.value = normalizeSettings({ ...settings.value, ...partial });
    supertonicEngine.setCustomVoices(settings.value.customVoices);
    persist();
  }

  function isEnabledForSession(sessionId: string | null | undefined): boolean {
    if (!sessionId) return false;
    const map = settings.value.sessionEnabled;
    if (Object.prototype.hasOwnProperty.call(map, sessionId)) {
      return Boolean(map[sessionId]);
    }
    return settings.value.defaultEnabled;
  }

  async function setSessionEnabled(sessionId: string, enabled: boolean) {
    const next = {
      ...settings.value.sessionEnabled,
      [sessionId]: enabled,
    };
    patchSettings({ sessionEnabled: next });
    if (enabled) {
      try {
        await ensureEngine();
      } catch (error) {
        console.warn("Failed to bootstrap Supertonic TTS:", error);
      }
    } else {
      streamer.stop(sessionId);
    }
  }

  async function ensureEngine() {
    await supertonicEngine.ensureReady({
      customVoices: settings.value.customVoices,
    });
    backend.value = supertonicEngine.getBackend();
  }

  function beginAssistantSpeech(sessionId: string, messageId: string) {
    if (!isEnabledForSession(sessionId)) return;
    streamer.begin(sessionId, messageId);
  }

  function pushAssistantDelta(
    sessionId: string,
    messageId: string,
    delta: string,
  ) {
    if (!isEnabledForSession(sessionId)) return;
    streamer.pushDelta(sessionId, messageId, delta);
  }

  function finalizeAssistantSpeech(
    sessionId: string,
    messageId: string,
    fullText?: string,
  ) {
    if (!isEnabledForSession(sessionId)) return;
    streamer.finalize(sessionId, messageId, fullText);
  }

  function stopSession(sessionId?: string) {
    streamer.stop(sessionId);
  }

  function importCustomVoice(name: string, style: unknown): string {
    if (!isValidVoiceStylePayload(style)) {
      throw new Error(
        "Invalid Voice Builder JSON. Expected style_ttl and style_dp tensors.",
      );
    }
    const id = `custom_${Date.now().toString(36)}_${Math.random()
      .toString(36)
      .slice(2, 7)}`;
    const entry: CustomVoiceStyle = {
      id,
      name: name.trim() || "Custom voice",
      style,
      createdAt: new Date().toISOString(),
    };
    const customVoices = [...settings.value.customVoices, entry];
    patchSettings({ customVoices, voiceId: id });
    return id;
  }

  function removeCustomVoice(id: string) {
    const customVoices = settings.value.customVoices.filter((v) => v.id !== id);
    const voiceId =
      settings.value.voiceId === id ? "M1" : settings.value.voiceId;
    patchSettings({ customVoices, voiceId });
  }

  const voiceOptions = computed(() => {
    const presets = PRESET_VOICE_IDS.map((id) => ({
      id,
      label: id.startsWith("M") ? `Male ${id.slice(1)} (${id})` : `Female ${id.slice(1)} (${id})`,
      value: id,
    }));
    const customs = settings.value.customVoices.map((v) => ({
      id: v.id,
      label: `${v.name} (custom)`,
      value: v.id,
    }));
    return [...presets, ...customs];
  });

  const statusLabel = computed(() => {
    switch (engineStatus.value) {
      case "downloading":
        return progress.value
          ? `Downloading ${progress.value.file || "models"} (${progress.value.current}/${progress.value.total})`
          : "Downloading Supertonic…";
      case "loading":
        return progress.value?.stage === "session"
          ? `Loading ${progress.value.file}`
          : "Loading Supertonic…";
      case "ready":
        return backend.value
          ? `Ready (${backend.value.toUpperCase()})`
          : "Ready";
      case "error":
        return engineError.value || "TTS model error";
      default:
        return "Idle";
    }
  });

  return {
    settings,
    engineStatus,
    engineError,
    progress,
    backend,
    statusLabel,
    voiceOptions,
    isEnabledForSession,
    setSessionEnabled,
    ensureEngine,
    patchSettings,
    importCustomVoice,
    removeCustomVoice,
    beginAssistantSpeech,
    pushAssistantDelta,
    finalizeAssistantSpeech,
    stopSession,
    resetSettings: () => {
      settings.value = defaultSupertonicTtsSettings();
      persist();
      stopSession();
    },
  };
});
