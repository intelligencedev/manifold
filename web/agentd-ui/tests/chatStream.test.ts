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

function makeStartRunResponse(runId = "run-1", sessionId = "abc") {
  return new Response(
    JSON.stringify({
      run_id: runId,
      session_id: sessionId,
      user_message_id: "user-1",
      assistant_message_id: "assistant-1",
      status: "running",
    }),
    {
      status: 200,
      headers: { "Content-Type": "application/json" },
    },
  );
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

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeStartRunResponse())
      .mockResolvedValueOnce(response);
    const received: ChatStreamEvent[] = [];

    await streamAgentRun({
      prompt: "test",
      sessionId: "abc",
      fetchImpl: fetchMock,
      onEvent: (event) => received.push(event),
    });

    expect(received).toEqual([
      { type: "run_started", run_id: "run-1", session_id: "abc" },
      { type: "delta", data: "Hello" },
      { type: "final", data: "Hello world" },
    ]);
  });

  it("handles non-streaming JSON responses", async () => {
    const response = new Response(JSON.stringify({ result: "done" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeStartRunResponse("run-json", ""))
      .mockResolvedValueOnce(response);
    const received: ChatStreamEvent[] = [];

    await streamAgentRun({
      prompt: "hello",
      onEvent: (event) => received.push(event),
      fetchImpl: fetchMock,
    });

    expect(received).toEqual([
      { type: "run_started", run_id: "run-json", session_id: "" },
      { type: "final", data: "done" },
    ]);
  });

  it("preserves duration from non-streaming JSON responses", async () => {
    const response = new Response(
      JSON.stringify({ result: "done", durationMs: 12437 }),
      {
        status: 200,
        headers: { "Content-Type": "application/json" },
      },
    );

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeStartRunResponse("run-duration", ""))
      .mockResolvedValueOnce(response);
    const received: ChatStreamEvent[] = [];

    await streamAgentRun({
      prompt: "hello",
      onEvent: (event) => received.push(event),
      fetchImpl: fetchMock,
    });

    expect(received).toEqual([
      { type: "run_started", run_id: "run-duration", session_id: "" },
      { type: "final", data: "done", durationMs: 12437 },
    ]);
  });

  it("includes memory settings in the run payload", async () => {
    const response = new Response(JSON.stringify({ result: "done" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeStartRunResponse())
      .mockResolvedValueOnce(response);

    await streamAgentRun({
      prompt: "hello",
      sessionId: "abc",
      memoryEnabled: false,
      onEvent: () => {},
      fetchImpl: fetchMock,
    });

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(init.body))).toMatchObject({
      prompt: "hello",
      session_id: "abc",
      memory_enabled: false,
    });
  });

  it("routes team runs with the team query parameter", async () => {
    const response = new Response(JSON.stringify({ result: "done" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(makeStartRunResponse())
      .mockResolvedValueOnce(response);

    await streamAgentRun({
      prompt: "hello",
      teamName: "ops",
      onEvent: () => {},
      fetchImpl: fetchMock,
    });

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "/api/chat/runs?team=ops",
    );
  });
});
