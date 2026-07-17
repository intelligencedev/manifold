import { describe, expect, it } from "vitest";
import {
  allRequiredAssetPaths,
  huggingfaceUrl,
  PRESET_VOICE_IDS,
} from "@/lib/tts/supertonic/assets";
import {
  isValidVoiceStylePayload,
  normalizeSettings,
} from "@/lib/tts/supertonic/settings";

describe("supertonic assets metadata", () => {
  it("lists onnx + presets", () => {
    const paths = allRequiredAssetPaths();
    expect(paths.some((p) => p.endsWith("vocoder.onnx"))).toBe(true);
    for (const id of PRESET_VOICE_IDS) {
      expect(paths).toContain(`voice_styles/${id}.json`);
    }
  });

  it("builds huggingface urls", () => {
    expect(huggingfaceUrl("onnx/tts.json")).toContain(
      "huggingface.co/Supertone/supertonic-3/resolve/main/onnx/tts.json",
    );
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
