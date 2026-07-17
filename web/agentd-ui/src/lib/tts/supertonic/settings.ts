import type { PresetVoiceId } from "./assets";
import { PRESET_VOICE_IDS } from "./assets";

export const TTS_SETTINGS_STORAGE_KEY = "manifold.ui.supertonic-tts";

export type CustomVoiceStyle = {
  id: string;
  name: string;
  /** Voice Builder JSON (style_ttl + style_dp) */
  style: unknown;
  createdAt: string;
};

export type SupertonicTtsSettings = {
  /** Global default for new chat sessions */
  defaultEnabled: boolean;
  voiceId: string;
  language: string;
  totalSteps: number;
  speed: number;
  customVoices: CustomVoiceStyle[];
  /** sessionId -> enabled */
  sessionEnabled: Record<string, boolean>;
};

const DEFAULT_SETTINGS: SupertonicTtsSettings = {
  defaultEnabled: false,
  voiceId: "M1",
  language: "en",
  totalSteps: 5,
  speed: 1.05,
  customVoices: [],
  sessionEnabled: {},
};

function isClient(): boolean {
  return typeof window !== "undefined" && typeof localStorage !== "undefined";
}

function clamp(n: number, min: number, max: number): number {
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, n));
}

export function defaultSupertonicTtsSettings(): SupertonicTtsSettings {
  return {
    ...DEFAULT_SETTINGS,
    customVoices: [],
    sessionEnabled: {},
  };
}

export function isPresetVoiceId(id: string): id is PresetVoiceId {
  return (PRESET_VOICE_IDS as readonly string[]).includes(id);
}

export function loadSupertonicTtsSettings(): SupertonicTtsSettings {
  if (!isClient()) return defaultSupertonicTtsSettings();
  try {
    const raw = localStorage.getItem(TTS_SETTINGS_STORAGE_KEY);
    if (!raw) return defaultSupertonicTtsSettings();
    const parsed = JSON.parse(raw) as Partial<SupertonicTtsSettings>;
    return normalizeSettings(parsed);
  } catch {
    return defaultSupertonicTtsSettings();
  }
}

export function saveSupertonicTtsSettings(
  settings: SupertonicTtsSettings,
): void {
  if (!isClient()) return;
  localStorage.setItem(
    TTS_SETTINGS_STORAGE_KEY,
    JSON.stringify(normalizeSettings(settings)),
  );
}

export function normalizeSettings(
  input: Partial<SupertonicTtsSettings> | null | undefined,
): SupertonicTtsSettings {
  const base = defaultSupertonicTtsSettings();
  if (!input || typeof input !== "object") return base;

  const voiceId =
    typeof input.voiceId === "string" && input.voiceId.trim()
      ? input.voiceId.trim()
      : base.voiceId;
  const language =
    typeof input.language === "string" && input.language.trim()
      ? input.language.trim()
      : base.language;
  const totalSteps = clamp(
    typeof input.totalSteps === "number" ? input.totalSteps : base.totalSteps,
    1,
    20,
  );
  const speed = clamp(
    typeof input.speed === "number" ? input.speed : base.speed,
    0.7,
    1.6,
  );
  const customVoices = Array.isArray(input.customVoices)
    ? input.customVoices
        .filter(
          (v): v is CustomVoiceStyle =>
            Boolean(
              v &&
                typeof v === "object" &&
                typeof (v as CustomVoiceStyle).id === "string" &&
                typeof (v as CustomVoiceStyle).name === "string" &&
                (v as CustomVoiceStyle).style != null,
            ),
        )
        .map((v) => ({
          id: v.id,
          name: v.name,
          style: v.style,
          createdAt:
            typeof v.createdAt === "string"
              ? v.createdAt
              : new Date().toISOString(),
        }))
    : [];

  const sessionEnabled: Record<string, boolean> = {};
  if (input.sessionEnabled && typeof input.sessionEnabled === "object") {
    for (const [sessionId, enabled] of Object.entries(input.sessionEnabled)) {
      if (typeof enabled === "boolean") sessionEnabled[sessionId] = enabled;
    }
  }

  return {
    defaultEnabled: Boolean(input.defaultEnabled),
    voiceId,
    language,
    totalSteps,
    speed,
    customVoices,
    sessionEnabled,
  };
}

export function isValidVoiceStylePayload(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const style = value as Record<string, unknown>;
  const ttl = style.style_ttl as Record<string, unknown> | undefined;
  const dp = style.style_dp as Record<string, unknown> | undefined;
  if (!ttl || !dp) return false;
  if (!Array.isArray(ttl.dims) || !Array.isArray(ttl.data)) return false;
  if (!Array.isArray(dp.dims) || !Array.isArray(dp.data)) return false;
  return true;
}
