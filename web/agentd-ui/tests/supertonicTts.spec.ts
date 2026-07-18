import { describe, expect, it } from "vitest";
import { PRESET_VOICE_IDS } from "@/lib/tts/supertonic/constants";
import {
  isValidVoiceStylePayload,
  normalizeSettings,
} from "@/lib/tts/supertonic/settings";

describe("supertonic presets", () => {
  it("exposes the ten preset voice ids", () => {
    expect(PRESET_VOICE_IDS).toEqual([
      "M1", "M2", "M3", "M4", "M5", "F1", "F2", "F3", "F4", "F5",
    ]);
  });
});

describe("supertonic settings", () => {
  it("normalizes partial settings", () => {
    const s = normalizeSettings({
      voiceId: "F2",
      totalSteps: 99,
      speed: 0.1,
      defaultEnabled: true,
    });
    expect(s.voiceId).toBe("F2");
    expect(s.totalSteps).toBe(20);
    expect(s.speed).toBe(0.7);
    expect(s.defaultEnabled).toBe(true);
  });

  it("validates voice builder payload", () => {
    expect(isValidVoiceStylePayload({})).toBe(false);
    expect(
      isValidVoiceStylePayload({
        style_ttl: { dims: [1, 1, 2], data: [[0, 1]] },
        style_dp: { dims: [1, 1, 2], data: [[0, 1]] },
      }),
    ).toBe(true);
  });
});
