import { describe, expect, it, vi } from "vitest";
import {
  extractEventPayload,
  streamAgentRun,
  type ChatStreamEvent,
} from "@/api/chat";

const encoder = new TextEncoder();

function makeSSEStream(chunks: string[]) {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe("extractEventPayload", () => {
  it("parses a simple SSE payload", () => {
    const raw = 'data: {"type":"delta","data":"hi"}';
    const event = extractEventPayload(raw);
    expect(event).toEqual({ type: "delta", data: "hi" });
  });

  it("returns null for invalid JSON", () => {
    const event = extractEventPayload("data: not-json");
    expect(event).toBeNull();
  });

  it("returns null when type missing", () => {
    const event = extractEventPayload('data: {"foo": "bar"}');
    expect(event).toBeNull();
  });
});

describe("streamAgentRun", () => {
  it("emits SSE events from the stream", async () => {
    const chunks = [
      'data: {"type":"delta","data":"Hello"}\n\n',
      'data: {"type":"final","data":"Hello world"}\n\n',
    ];
    const response = new Response(makeSSEStream(chunks), {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    });

    const fetchMock = vi.fn().mockResolvedValue(response);
    const received: ChatStreamEvent[] = [];

    await streamAgentRun({
      prompt: "test",
      sessionId: "abc",
      fetchImpl: fetchMock,
      onEvent: (event) => received.push(event),
    });

    expect(received).toEqual([
      { type: "delta", data: "Hello" },
      { type: "final", data: "Hello world" },
    ]);
  });

  it("handles non-streaming JSON responses", async () => {
    const response = new Response(JSON.stringify({ result: "done" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });

    const fetchMock = vi.fn().mockResolvedValue(response);
    const received: ChatStreamEvent[] = [];

    await streamAgentRun({
      prompt: "hello",
      onEvent: (event) => received.push(event),
      fetchImpl: fetchMock,
    });

    expect(received).toEqual([{ type: "final", data: "done" }]);
  });
});
