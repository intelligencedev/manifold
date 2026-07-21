export type NoiseSuppressionMode = "automatic" | "standard" | "strong";

export type RealtimeAudioSettings = {
  inputDeviceId: string;
  suppressionMode: NoiseSuppressionMode;
  autoGainControl: boolean;
};

export type RealtimeCaptureCapabilities = {
  echoCancellation: boolean;
  noiseSuppression: boolean;
  autoGainControl: boolean;
  voiceIsolation: boolean;
  deviceSelection: boolean;
};

export type AppliedRealtimeCaptureSettings = {
  deviceId: string;
  sampleRate?: number;
  channelCount?: number;
  echoCancellation?: boolean;
  noiseSuppression?: boolean;
  autoGainControl?: boolean;
  voiceIsolation?: boolean;
};

const STORAGE_KEY = "manifold.realtime.audio.v1";

export const defaultRealtimeAudioSettings: RealtimeAudioSettings = {
  inputDeviceId: "",
  suppressionMode: "automatic",
  autoGainControl: false,
};

export function loadRealtimeAudioSettings(): RealtimeAudioSettings {
  if (typeof window === "undefined") return { ...defaultRealtimeAudioSettings };
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
    return normalizeRealtimeAudioSettings(stored);
  } catch {
    return { ...defaultRealtimeAudioSettings };
  }
}

export function saveRealtimeAudioSettings(settings: RealtimeAudioSettings) {
  if (typeof window === "undefined") return;
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify(normalizeRealtimeAudioSettings(settings)),
  );
}

export function normalizeRealtimeAudioSettings(
  value: unknown,
): RealtimeAudioSettings {
  const candidate =
    value && typeof value === "object"
      ? (value as Partial<RealtimeAudioSettings>)
      : {};
  const suppressionMode = isNoiseSuppressionMode(candidate.suppressionMode)
    ? candidate.suppressionMode
    : defaultRealtimeAudioSettings.suppressionMode;
  return {
    inputDeviceId:
      typeof candidate.inputDeviceId === "string"
        ? candidate.inputDeviceId
        : "",
    suppressionMode,
    autoGainControl: candidate.autoGainControl === true,
  };
}

export function detectRealtimeCaptureCapabilities(
  mediaDevices: MediaDevices | undefined = typeof navigator === "undefined"
    ? undefined
    : navigator.mediaDevices,
): RealtimeCaptureCapabilities {
  const supported = mediaDevices?.getSupportedConstraints?.() || {};
  const extended = supported as MediaTrackSupportedConstraints & {
    voiceIsolation?: boolean;
  };
  return {
    echoCancellation: Boolean(supported.echoCancellation),
    noiseSuppression: Boolean(supported.noiseSuppression),
    autoGainControl: Boolean(supported.autoGainControl),
    voiceIsolation: Boolean(extended.voiceIsolation),
    deviceSelection: Boolean(supported.deviceId),
  };
}

export function buildRealtimeAudioConstraints(
  settings: RealtimeAudioSettings,
  capabilities: RealtimeCaptureCapabilities,
): MediaTrackConstraints {
  const strong = settings.suppressionMode === "strong";
  const constraints: MediaTrackConstraints & { voiceIsolation?: boolean } = {
    channelCount: { ideal: 2 },
    latency: { ideal: 0.01 },
  };
  if (capabilities.deviceSelection && settings.inputDeviceId) {
    constraints.deviceId = { exact: settings.inputDeviceId };
  }
  if (capabilities.echoCancellation) constraints.echoCancellation = true;
  if (capabilities.noiseSuppression) {
    // Only standard mode uses the browser's generic denoiser. Automatic uses
    // voice isolation when available and RNNoise otherwise; strong always
    // uses RNNoise. This prevents two denoisers from damaging the same signal.
    constraints.noiseSuppression =
      settings.suppressionMode === "standard" && !strong;
  }
  if (capabilities.autoGainControl) {
    constraints.autoGainControl = settings.autoGainControl;
  }
  if (capabilities.voiceIsolation) {
    constraints.voiceIsolation =
      settings.suppressionMode === "automatic" && !strong;
  }
  return constraints;
}

export function readAppliedCaptureSettings(
  track: MediaStreamTrack,
): AppliedRealtimeCaptureSettings {
  const applied = track.getSettings() as MediaTrackSettings & {
    voiceIsolation?: boolean;
  };
  return {
    deviceId: applied.deviceId || "",
    sampleRate: applied.sampleRate,
    channelCount: applied.channelCount,
    echoCancellation: applied.echoCancellation,
    noiseSuppression: applied.noiseSuppression,
    autoGainControl: applied.autoGainControl,
    voiceIsolation: applied.voiceIsolation,
  };
}

export function shouldUseRnnoise(
  settings: RealtimeAudioSettings,
  applied: AppliedRealtimeCaptureSettings | null,
): boolean {
  if (settings.suppressionMode === "strong") return true;
  if (settings.suppressionMode === "standard") return false;
  return applied?.voiceIsolation !== true;
}

function isNoiseSuppressionMode(value: unknown): value is NoiseSuppressionMode {
  return value === "automatic" || value === "standard" || value === "strong";
}
