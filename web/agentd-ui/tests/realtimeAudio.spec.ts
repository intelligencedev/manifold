import { describe, expect, it } from "vitest";
import {
  AdaptiveVoiceActivityDetector,
  encodePCM16WAV,
  mergeAudioFrames,
  resampleLinear,
} from "@/lib/realtime/audio";

describe("AdaptiveVoiceActivityDetector", () => {
  it("emits stable speech boundaries instead of reacting to one frame", () => {
    const detector = new AdaptiveVoiceActivityDetector({
      startFrames: 3,
      endFrames: 4,
    });
    const silence = new Float32Array(320);
    const speech = new Float32Array(320).fill(0.08);

    expect(detector.process(silence).event).toBeNull();
    expect(detector.process(speech).event).toBeNull();
    expect(detector.process(speech).event).toBeNull();
    expect(detector.process(speech).event).toBe("speech-start");
    expect(detector.process(silence).event).toBeNull();
    expect(detector.process(silence).event).toBeNull();
    expect(detector.process(silence).event).toBeNull();
    expect(detector.process(silence).event).toBe("speech-end");
  });
});

describe("realtime audio conversion", () => {
  it("merges and resamples capture frames", () => {
    const merged = mergeAudioFrames([
      new Float32Array([0, 1]),
      new Float32Array([0, -1]),
    ]);
    expect(Array.from(merged)).toEqual([0, 1, 0, -1]);

    const downsampled = resampleLinear(merged, 32_000, 16_000);
    expect(Array.from(downsampled)).toEqual([0, 0]);
  });

  it("encodes a valid mono PCM16 WAV payload", async () => {
    const wav = encodePCM16WAV(new Float32Array([-1, 0, 1]), 16_000);
    const bytes = new Uint8Array(await readBlob(wav));
    const view = new DataView(bytes.buffer);

    expect(new TextDecoder().decode(bytes.slice(0, 4))).toBe("RIFF");
    expect(new TextDecoder().decode(bytes.slice(8, 12))).toBe("WAVE");
    expect(view.getUint16(22, true)).toBe(1);
    expect(view.getUint32(24, true)).toBe(16_000);
    expect(view.getUint16(34, true)).toBe(16);
    expect(view.getInt16(44, true)).toBe(-32_768);
    expect(view.getInt16(48, true)).toBe(32_767);
  });
});

function readBlob(blob: Blob): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.readAsArrayBuffer(blob);
  });
}
