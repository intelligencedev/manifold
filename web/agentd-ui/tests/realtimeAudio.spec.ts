import { describe, expect, it } from "vitest";
import {
  AdaptiveVoiceActivityDetector,
  encodePCM16WAV,
  estimateSpeechProbability,
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

  it("rejects loud non-speech frames when the cleaned signal has low voice probability", () => {
    const detector = new AdaptiveVoiceActivityDetector({ startFrames: 2 });
    const loudNoise = new Float32Array(320).fill(0.09);

    const first = detector.process(loudNoise, {
      speechProbability: 0.12,
      noiseFloor: 0.004,
      snrDb: 24,
    });
    const second = detector.process(loudNoise, {
      speechProbability: 0.12,
      noiseFloor: 0.004,
      snrDb: 24,
    });

    expect(first.rejectedNoise).toBe(true);
    expect(second.event).toBeNull();
    expect(second.speaking).toBe(false);
  });

  it("calibrates the ambient floor without opening a speech turn", () => {
    const detector = new AdaptiveVoiceActivityDetector({ startFrames: 1 });
    const roomNoise = new Float32Array(320).fill(0.025);
    detector.calibrate(3);

    expect(detector.process(roomNoise).event).toBeNull();
    expect(detector.process(roomNoise).event).toBeNull();
    expect(detector.process(roomNoise).event).toBeNull();
    expect(detector.calibrationRemaining).toBe(0);
    expect(detector.ambientNoiseFloor).toBeGreaterThan(0.01);
  });

  it("scores sustained voice-like energy above an impulse", () => {
    const voice = new Float32Array(480);
    for (let index = 0; index < voice.length; index += 1) {
      voice[index] = Math.sin((index / 48_000) * Math.PI * 2 * 180) * 0.08;
    }
    const impulse = new Float32Array(480);
    impulse[20] = 1;

    expect(estimateSpeechProbability(voice, 18)).toBeGreaterThan(
      estimateSpeechProbability(impulse, 18),
    );
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
