import { describe, expect, it } from "vitest";
import captureWorkletSource from "../public/realtime-capture-worklet.js?raw";
import {
  closeSpeechFrame,
  corpusFrameLength,
  fanFrame,
  keyboardFrame,
} from "./fixtures/realtimeNoiseCorpus";

type CaptureMessage = {
  type: string;
  samples: Float32Array;
  sequence: number;
  channels: number;
  beamformed: boolean;
  speechProbability: number;
  noiseFloor: number;
  snrDb: number;
  rejectedNoise: boolean;
  processingMs: number;
};

type CaptureProcessor = {
  port: MockPort;
  pending: Float32Array;
  mixBuffer: Float32Array;
  process(inputs: Float32Array[][], outputs: Float32Array[][]): boolean;
};

type CaptureProcessorConstructor = new () => CaptureProcessor;

class MockPort {
  onmessage: ((event: MessageEvent) => void) | null = null;
  messages: CaptureMessage[] = [];

  postMessage(message: CaptureMessage) {
    this.messages.push({
      ...message,
      samples: new Float32Array(message.samples),
    });
  }
}

class MockAudioWorkletProcessor {
  port = new MockPort();
}

describe("realtime capture worklet", () => {
  it("calibrates, identifies close speech, and rejects keyboard impulses", () => {
    const processor = createProcessor();
    for (let index = 0; index < 15; index += 1) {
      feedMono(processor, fanFrame(index));
    }

    expect(processor.port.messages).toHaveLength(15);
    expect(
      processor.port.messages.every(
        (message) => message.speechProbability === 0,
      ),
    ).toBe(true);

    feedMono(processor, closeSpeechFrame(16));
    const speech = processor.port.messages.at(-1)!;
    expect(speech.speechProbability).toBeGreaterThan(0.48);
    expect(speech.snrDb).toBeGreaterThan(6);

    feedMono(processor, keyboardFrame(20));
    const keyboard = processor.port.messages.at(-1)!;
    expect(keyboard.speechProbability).toBeLessThan(0.48);
    expect(keyboard.rejectedNoise).toBe(true);
    expect(keyboard.processingMs).toBeGreaterThanOrEqual(0);
  });

  it("aligns a coherent two-microphone signal before capture", () => {
    const processor = createProcessor();
    const primary = closeSpeechFrame(0);
    const secondary = new Float32Array(corpusFrameLength);
    for (let index = 6; index < secondary.length; index += 1) {
      secondary[index] = primary[index - 6] * 0.72;
    }

    feedStereo(processor, primary, secondary);
    const message = processor.port.messages.at(-1)!;

    expect(message.channels).toBe(2);
    expect(message.beamformed).toBe(true);
    expect(message.samples).toHaveLength(corpusFrameLength);
    expect(Array.from(message.samples)).not.toEqual(Array.from(primary));
  });

  it("emits exact 20 ms frames across browser render quanta", () => {
    const processor = createProcessor();
    const input = closeSpeechFrame(0);
    for (let offset = 0; offset < input.length; offset += 128) {
      const quantum = new Float32Array(128);
      quantum.set(input.subarray(offset, Math.min(input.length, offset + 128)));
      feedMono(processor, quantum);
    }

    expect(processor.port.messages).toHaveLength(1);
    expect(processor.port.messages[0].samples).toHaveLength(960);
    expect(processor.port.messages[0].sequence).toBe(0);
  });

  it("reuses its capture buffers throughout a sustained call", () => {
    const processor = createProcessor();
    const pending = processor.pending;
    const mixBuffer = processor.mixBuffer;
    const first = closeSpeechFrame(0).subarray(0, 128);
    const second = closeSpeechFrame(1).subarray(0, 128);

    for (let index = 0; index < 10_000; index += 1) {
      feedStereo(processor, first, second);
      processor.port.messages.length = 0;
    }

    expect(processor.pending).toBe(pending);
    expect(processor.mixBuffer).toBe(mixBuffer);
  });
});

function createProcessor(): CaptureProcessor {
  let Processor: CaptureProcessorConstructor | undefined;
  const evaluate = new Function(
    "AudioWorkletProcessor",
    "sampleRate",
    "registerProcessor",
    captureWorkletSource,
  );
  evaluate(
    MockAudioWorkletProcessor,
    48_000,
    (name: string, constructor: CaptureProcessorConstructor) => {
      expect(name).toBe("manifold-realtime-capture");
      Processor = constructor;
    },
  );
  if (!Processor) throw new Error("Capture worklet did not register");
  return new Processor();
}

function feedMono(processor: CaptureProcessor, frame: Float32Array) {
  const output = new Float32Array(frame.length);
  expect(processor.process([[frame]], [[output]])).toBe(true);
  expect(output.every((sample) => sample === 0)).toBe(true);
}

function feedStereo(
  processor: CaptureProcessor,
  first: Float32Array,
  second: Float32Array,
) {
  const output = new Float32Array(first.length);
  expect(processor.process([[first, second]], [[output]])).toBe(true);
}
