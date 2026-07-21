import { describe, expect, it } from "vitest";
import { AdaptiveVoiceActivityDetector } from "@/lib/realtime/audio";
import {
  buildRealtimeNoiseCorpus,
  closeSpeechFrame,
  quietFrame,
} from "./fixtures/realtimeNoiseCorpus";

describe("realtime noisy-room corpus", () => {
  for (const fixture of buildRealtimeNoiseCorpus()) {
    it(`keeps stable turn boundaries in a ${fixture.name}`, () => {
      const detector = new AdaptiveVoiceActivityDetector();
      detector.calibrate(fixture.calibration.length);
      for (const frame of fixture.calibration) detector.process(frame);

      let starts = 0;
      let ends = 0;
      let falseStarts = 0;
      let missedVoicedFrames = 0;
      let expectedVoicedFrames = 0;
      for (const frame of fixture.frames) {
        const update = detector.process(frame.samples);
        if (update.event === "speech-start") {
          starts += 1;
          if (!frame.speech) falseStarts += 1;
        }
        if (update.event === "speech-end") ends += 1;
        if (frame.speech) {
          expectedVoicedFrames += 1;
          if (!update.speaking) missedVoicedFrames += 1;
        }
      }

      expect(starts).toBe(fixture.expectedTurns);
      expect(ends).toBe(fixture.expectedTurns);
      expect(falseStarts).toBe(0);
      if (expectedVoicedFrames > 0) {
        // Six 20 ms confirmation frames are intentionally held before a turn
        // opens, so the acceptable miss budget includes detector attack time.
        expect(missedVoicedFrames / expectedVoicedFrames).toBeLessThan(0.25);
      }
    });
  }

  it("holds stable state for a simulated sixty-minute call", () => {
    const detector = new AdaptiveVoiceActivityDetector({
      startFrames: 3,
      endFrames: 4,
    });
    const noise = new Float32Array(16).fill(0.004);
    const speech = new Float32Array(16).fill(0.08);
    const framesInOneHour = 60 * 60 * 50;
    let starts = 0;
    let ends = 0;
    const startedAt = performance.now();

    for (let index = 0; index < framesInOneHour; index += 1) {
      const cycleFrame = index % 1_500;
      const voiced = cycleFrame >= 100 && cycleFrame < 125;
      const update = detector.process(voiced ? speech : noise, {
        speechProbability: voiced ? 0.92 : 0.08,
        noiseFloor: 0.004,
        snrDb: voiced ? 24 : 0,
      });
      if (update.event === "speech-start") starts += 1;
      if (update.event === "speech-end") ends += 1;
    }

    const elapsed = performance.now() - startedAt;
    expect(starts).toBe(120);
    expect(ends).toBe(120);
    expect(detector.ambientNoiseFloor).toBeGreaterThan(0);
    expect(Number.isFinite(detector.ambientNoiseFloor)).toBe(true);
    // This leaves a deliberately broad CI-safe budget while catching an
    // accidental super-linear or per-call blocking regression.
    expect(elapsed / framesInOneHour).toBeLessThan(0.1);
  });

  it("opens a barge-in turn within 120 ms", () => {
    const detector = new AdaptiveVoiceActivityDetector();
    detector.calibrate(15);
    for (let index = 0; index < 15; index += 1) {
      detector.process(quietFrame(index));
    }

    let startFrame = 0;
    for (let index = 0; index < 10; index += 1) {
      if (detector.process(closeSpeechFrame(index)).event === "speech-start") {
        startFrame = index + 1;
        break;
      }
    }

    expect(startFrame).toBeGreaterThan(0);
    expect(startFrame * 20).toBeLessThanOrEqual(120);
  });

  it("computes transcript word error rate for Moonshine corpus reports", () => {
    expect(wordErrorRate("Turn on the lights.", "turn the light")).toBeCloseTo(
      0.5,
    );
    expect(wordErrorRate("Hello, Manifold!", "hello manifold")).toBe(0);
  });
});

function wordErrorRate(reference: string, hypothesis: string): number {
  const expected = words(reference);
  const actual = words(hypothesis);
  if (!expected.length) return actual.length ? 1 : 0;
  const previous = Array.from(
    { length: actual.length + 1 },
    (_, index) => index,
  );
  for (
    let expectedIndex = 1;
    expectedIndex <= expected.length;
    expectedIndex += 1
  ) {
    let diagonal = previous[0];
    previous[0] = expectedIndex;
    for (let actualIndex = 1; actualIndex <= actual.length; actualIndex += 1) {
      const above = previous[actualIndex];
      previous[actualIndex] = Math.min(
        previous[actualIndex] + 1,
        previous[actualIndex - 1] + 1,
        diagonal +
          (expected[expectedIndex - 1] === actual[actualIndex - 1] ? 0 : 1),
      );
      diagonal = above;
    }
  }
  return previous[actual.length] / expected.length;
}

function words(value: string): string[] {
  return value.toLowerCase().match(/[\p{L}\p{N}']+/gu) || [];
}
