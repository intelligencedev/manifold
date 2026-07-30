import { beforeEach, describe, expect, it } from "vitest";
import {
  buildRealtimeAudioConstraints,
  defaultRealtimeAudioSettings,
  detectRealtimeCaptureCapabilities,
  loadRealtimeAudioSettings,
  saveRealtimeAudioSettings,
  shouldUseRnnoise,
} from "@/lib/realtime/settings";

const capabilities = {
  echoCancellation: true,
  noiseSuppression: true,
  autoGainControl: true,
  voiceIsolation: true,
  deviceSelection: true,
};

describe("realtime audio settings", () => {
  beforeEach(() => localStorage.clear());

  it("persists normalized microphone preferences", () => {
    saveRealtimeAudioSettings({
      inputDeviceId: "near-mic",
      suppressionMode: "strong",
      autoGainControl: true,
    });

    expect(loadRealtimeAudioSettings()).toEqual({
      inputDeviceId: "near-mic",
      suppressionMode: "strong",
      autoGainControl: true,
    });
  });

  it("falls back to safe defaults for malformed preferences", () => {
    localStorage.setItem(
      "manifold.realtime.audio.v1",
      JSON.stringify({ suppressionMode: "maximum", autoGainControl: "yes" }),
    );

    expect(loadRealtimeAudioSettings()).toEqual(defaultRealtimeAudioSettings);
  });

  it("requests voice isolation automatically and avoids double suppression in strong mode", () => {
    const automatic = buildRealtimeAudioConstraints(
      {
        inputDeviceId: "near-mic",
        suppressionMode: "automatic",
        autoGainControl: false,
      },
      capabilities,
    ) as MediaTrackConstraints & { voiceIsolation?: boolean };
    expect(automatic).toMatchObject({
      deviceId: { exact: "near-mic" },
      echoCancellation: true,
      noiseSuppression: false,
      autoGainControl: false,
      voiceIsolation: true,
    });

    const strong = buildRealtimeAudioConstraints(
      { ...defaultRealtimeAudioSettings, suppressionMode: "strong" },
      capabilities,
    ) as MediaTrackConstraints & { voiceIsolation?: boolean };
    expect(strong.noiseSuppression).toBe(false);
    expect(strong.voiceIsolation).toBe(false);
  });

  it("uses RNNoise when strong mode is selected or native voice isolation is absent", () => {
    expect(
      shouldUseRnnoise(
        { ...defaultRealtimeAudioSettings, suppressionMode: "strong" },
        { deviceId: "mic", voiceIsolation: true },
      ),
    ).toBe(true);
    expect(
      shouldUseRnnoise(defaultRealtimeAudioSettings, {
        deviceId: "mic",
        voiceIsolation: true,
      }),
    ).toBe(false);
    expect(
      shouldUseRnnoise(defaultRealtimeAudioSettings, {
        deviceId: "mic",
        voiceIsolation: false,
      }),
    ).toBe(true);
  });

  it("detects optional browser capture capabilities", () => {
    const mediaDevices = {
      getSupportedConstraints: () => ({
        deviceId: true,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: false,
        voiceIsolation: true,
      }),
    } as unknown as MediaDevices;

    expect(detectRealtimeCaptureCapabilities(mediaDevices)).toEqual({
      deviceSelection: true,
      echoCancellation: true,
      noiseSuppression: true,
      autoGainControl: false,
      voiceIsolation: true,
    });
  });
});
