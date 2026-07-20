import { afterEach, describe, expect, it, vi } from "vitest";

const engineMock = vi.hoisted(() => ({
  ensureReady: vi.fn(async () => {}),
  synthesize: vi.fn(async () => ({
    samples: new Float32Array(0),
    sampleRate: 44100,
    durationSec: 0,
  })),
  synthesizeStream: vi.fn(),
}));

vi.mock("@/lib/tts/supertonic/serverEngine", () => ({
  serverEngine: engineMock,
}));

import { SupertonicStreamer } from "@/lib/tts/supertonic/streamer";

class FakeAudioContext {
  currentTime = 1;
  state = "running";
  destination = {};
  source = {
    buffer: null,
    connect: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
    onended: null,
  };

  createBuffer = vi.fn(() => ({
    duration: 1 / 44100,
    copyToChannel: vi.fn(),
  }));

  createBufferSource = vi.fn(() => this.source);
  resume = vi.fn(async () => {});
}

afterEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(window, "AudioContext");
});

describe("SupertonicStreamer", () => {
  it("schedules each streamed PCM chunk as soon as it arrives", async () => {
    const audioContext = new FakeAudioContext();
    Object.defineProperty(window, "AudioContext", {
      configurable: true,
      value: class {
        constructor() {
          return audioContext;
        }
      },
    });
    engineMock.synthesizeStream.mockImplementation(
      async (_options, onChunk) => {
        await onChunk(new Float32Array([0.25]), 44100);
        return {
          samples: new Float32Array(0),
          sampleRate: 44100,
          durationSec: 1 / 44100,
        };
      },
    );

    const streamer = new SupertonicStreamer({
      getSettings: () => ({
        language: "en",
        voiceId: "M1",
        totalSteps: 8,
        speed: 1,
        customVoices: [],
      }),
      isEnabledForSession: () => true,
    });

    streamer.begin("session", "message");
    streamer.pushDelta("session", "message", "Hello streaming world.");

    await vi.waitFor(() => {
      expect(engineMock.synthesizeStream).toHaveBeenCalledOnce();
      expect(audioContext.source.start).toHaveBeenCalledOnce();
    });
    expect(engineMock.synthesize).not.toHaveBeenCalled();

    streamer.stop();
  });

  it("flushes short clauses quickly and reports realtime playback state", async () => {
    vi.useFakeTimers();
    const audioContext = new FakeAudioContext();
    Object.defineProperty(window, "AudioContext", {
      configurable: true,
      value: class {
        constructor() {
          return audioContext;
        }
      },
    });
    engineMock.synthesizeStream.mockImplementation(
      async (_options, onChunk) => {
        await onChunk(new Float32Array([0.25]), 44100);
        return {
          samples: new Float32Array(0),
          sampleRate: 44100,
          durationSec: 1 / 44100,
        };
      },
    );
    const states: boolean[] = [];
    const streamer = new SupertonicStreamer({
      getSettings: () => ({
        language: "en",
        voiceId: "M1",
        totalSteps: 8,
        speed: 1,
        customVoices: [],
      }),
      isEnabledForSession: () => true,
      isRealtimeSession: () => true,
      onPlaybackStateChange: (_sessionId, speaking) => states.push(speaking),
    });

    streamer.begin("session", "message");
    streamer.pushDelta(
      "session",
      "message",
      "I can help you with that right now ",
    );
    await vi.advanceTimersByTimeAsync(150);
    await Promise.resolve();

    expect(engineMock.synthesizeStream).toHaveBeenCalledOnce();
    expect(states).toEqual([true]);
    audioContext.source.onended?.(new Event("ended"));
    expect(states).toEqual([true, false]);

    streamer.stop();
    vi.useRealTimers();
  });

  it("resets the playback cursor when barge-in cancels queued audio", async () => {
    const audioContext = new FakeAudioContext();
    audioContext.createBuffer.mockReturnValue({
      duration: 4,
      copyToChannel: vi.fn(),
    });
    Object.defineProperty(window, "AudioContext", {
      configurable: true,
      value: class {
        constructor() {
          return audioContext;
        }
      },
    });
    engineMock.synthesizeStream.mockImplementation(
      async (_options, onChunk) => {
        await onChunk(new Float32Array([0.25]), 44100);
        return {
          samples: new Float32Array(0),
          sampleRate: 44100,
          durationSec: 1 / 44100,
        };
      },
    );
    const streamer = new SupertonicStreamer({
      getSettings: () => ({
        language: "en",
        voiceId: "M1",
        totalSteps: 8,
        speed: 1,
        customVoices: [],
      }),
      isEnabledForSession: () => true,
    });

    streamer.begin("session", "message-1");
    streamer.pushDelta("session", "message-1", "This audio is interrupted.");
    await vi.waitFor(() =>
      expect(audioContext.source.start).toHaveBeenCalled(),
    );
    streamer.stop("session");

    audioContext.currentTime = 1.1;
    streamer.begin("session", "message-2");
    streamer.pushDelta("session", "message-2", "The reply starts promptly.");
    await vi.waitFor(() =>
      expect(audioContext.source.start).toHaveBeenCalledTimes(2),
    );

    expect(audioContext.source.start.mock.calls[1][0]).toBeCloseTo(1.12, 5);
    streamer.stop();
  });
});
