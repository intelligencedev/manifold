import { afterEach, describe, expect, it, vi } from "vitest";
import { serverEngine } from "@/lib/tts/supertonic/serverEngine";

function sseFrame(obj: unknown): string {
  return `data: ${JSON.stringify(obj)}\n\n`;
}

// Build a Response whose body streams the given SSE text in arbitrary byte
// slices, to exercise the frame-boundary buffering.
function streamingResponse(text: string, chunkSize = 7): Response {
  const bytes = new TextEncoder().encode(text);
  let pos = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (pos >= bytes.length) {
        controller.close();
        return;
      }
      controller.enqueue(bytes.slice(pos, pos + chunkSize));
      pos += chunkSize;
    },
  });
  return new Response(body, { status: 200 });
}

function pcm16Base64(samples: number[]): string {
  const pcm = new Int16Array(samples);
  const bytes = new Uint8Array(pcm.buffer);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("serverEngine.synthesize", () => {
  it("delivers the first PCM chunk before the SSE response finishes", async () => {
    const firstFrame = sseFrame({
      type: "tts_chunk",
      b64: pcm16Base64([0, 16384]),
      rate: 44100,
      bytes: 4,
    });
    let closeStream!: () => void;
    const response = new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(firstFrame));
          closeStream = () => {
            controller.enqueue(
              new TextEncoder().encode(sseFrame({ type: "done", rate: 44100 })),
            );
            controller.close();
          };
        },
      }),
      { status: 200 },
    );
    vi.spyOn(globalThis, "fetch").mockResolvedValue(response);

    let requestFinished = false;
    let deliverFirst!: () => void;
    const firstDelivered = new Promise<void>((resolve) => {
      deliverFirst = resolve;
    });
    const received: number[][] = [];
    const request = serverEngine
      .synthesizeStream(
        { text: "Hello world.", voiceId: "M1", lang: "en" },
        (samples) => {
          received.push(Array.from(samples));
          deliverFirst();
        },
      )
      .finally(() => {
        requestFinished = true;
      });

    await firstDelivered;
    expect(requestFinished).toBe(false);
    expect(received).toHaveLength(1);
    expect(received[0].map((v) => Math.round(v * 10) / 10)).toEqual([0, 0.5]);

    closeStream();
    await request;
  });

  it("decodes streamed tts_chunk PCM16 frames into float32 samples", async () => {
    const b64a = pcm16Base64([0, 16384, -16384]); // -> 0, 0.5, -0.5
    const b64b = pcm16Base64([32767]); //           -> ~1.0
    const sse =
      sseFrame({ type: "tts_chunk", b64: b64a, rate: 44100, bytes: 6 }) +
      sseFrame({ type: "tts_chunk", b64: b64b, rate: 44100, bytes: 2 }) +
      sseFrame({ type: "done", rate: 44100 });

    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(streamingResponse(sse));

    const result = await serverEngine.synthesize({
      text: "Hello.",
      voiceId: "M1",
      lang: "en",
      totalSteps: 5,
      speed: 1.1,
    });

    // Posted to /tts with the expected body.
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/tts");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      text: "Hello.",
      voice: "M1",
      lang: "en",
      totalSteps: 5,
      speed: 1.1,
    });

    expect(result.sampleRate).toBe(44100);
    expect(
      Array.from(result.samples).map((v) => Math.round(v * 100) / 100),
    ).toEqual([0, 0.5, -0.5, 1]);
  });

  it("returns empty samples on a non-OK response without throwing", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("nope", { status: 400 }),
    );
    const result = await serverEngine.synthesize({ text: "hi", voiceId: "M1" });
    expect(result.samples.length).toBe(0);
  });

  it("skips empty text", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");
    const result = await serverEngine.synthesize({ text: "   " });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.samples.length).toBe(0);
  });

  it("reports as an always-ready server backend", () => {
    expect(serverEngine.isReady()).toBe(true);
    expect(serverEngine.getBackend()).toBe("server");
    expect(serverEngine.getStatus()).toBe("ready");
  });
});
