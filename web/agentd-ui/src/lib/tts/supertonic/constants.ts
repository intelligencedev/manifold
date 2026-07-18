// Lightweight, dependency-free constants and types shared by the Supertonic TTS
// store/streamer/settings. Extracted so nothing needs to import the former
// in-browser onnxruntime-web engine (now replaced by the host-side serverEngine),
// keeping onnxruntime-web out of the bundle entirely.

export const PRESET_VOICE_IDS = [
  "M1",
  "M2",
  "M3",
  "M4",
  "M5",
  "F1",
  "F2",
  "F3",
  "F4",
  "F5",
] as const;

export type PresetVoiceId = (typeof PRESET_VOICE_IDS)[number];

// Languages Supertonic supports (mirrors the model's AVAILABLE_LANGS).
export const availableLanguages: string[] = [
  "en", "ko", "ja", "ar", "bg", "cs", "da", "de", "el", "es", "et", "fi",
  "fr", "hi", "hr", "hu", "id", "it", "lt", "lv", "nl", "pl", "pt", "ro",
  "ru", "sk", "sl", "sv", "tr", "uk", "vi", "na",
];

export type EngineStatus = "idle" | "downloading" | "loading" | "ready" | "error";

export type SupertonicLoadProgress =
  | { stage: "cache" | "download" | "done"; file: string; current: number; total: number; bytesReceived?: number; bytesTotal?: number }
  | { stage: "session"; file: string; current: number; total: number }
  | { stage: "voice"; file: string; current: number; total: number };

export type SynthesizeOptions = {
  text: string;
  lang?: string;
  voiceId?: string;
  totalSteps?: number;
  speed?: number;
  silenceDuration?: number;
  signal?: AbortSignal;
};

export type SynthesizeResult = {
  samples: Float32Array;
  sampleRate: number;
  durationSec: number;
};
