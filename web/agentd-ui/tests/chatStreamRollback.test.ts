import { describe, expect, it } from "vitest";
import { createChatStoreState } from "@/stores/chatStoreState";
import { handleStreamEvent } from "@/stores/chatStreamEvents";
import {
  emitAssistantRollback,
  onAssistantRollback,
} from "@/lib/tts/supertonic/speechBus";

const streamId = "stream-1";
const sessionId = "session-1";
const assistantId = "assistant-1";

function makeStreamingState() {
  const state = createChatStoreState();
  state.setStreamingState(sessionId, {
    streamId,
    runId: "run-1",
    controller: undefined,
  } as never);
  state.setMessages(sessionId, [
    {
      id: assistantId,
      role: "assistant",
      content: "",
      createdAt: "2026-07-18T00:00:00.000Z",
      streaming: true,
    },
  ]);
  return state;
}

function feed(state: ReturnType<typeof createChatStoreState>, event: object) {
  handleStreamEvent(
    state,
    { invalidateQueries: () => {} },
    event as never,
    sessionId,
    assistantId,
    streamId,
  );
}

describe("delta_rollback stream event", () => {
  it("truncates the streamed assistant content by count", () => {
    const state = makeStreamingState();

    feed(state, { type: "delta", data: "too soon" });
    expect(state.messagesBySession.value[sessionId][0].content).toBe("too soon");

    feed(state, { type: "delta_rollback", count: "too soon".length });
    expect(state.messagesBySession.value[sessionId][0].content).toBe("");

    feed(state, { type: "delta", data: "accepted text" });
    expect(state.messagesBySession.value[sessionId][0].content).toBe(
      "accepted text",
    );
  });

  it("ignores a rollback with a non-positive count", () => {
    const state = makeStreamingState();
    feed(state, { type: "delta", data: "keep me" });

    feed(state, { type: "delta_rollback", count: 0 });

    expect(state.messagesBySession.value[sessionId][0].content).toBe("keep me");
  });
});

describe("speechBus rollback", () => {
  it("delivers rollback notifications to listeners", () => {
    const received: Array<[string, string, number]> = [];
    const off = onAssistantRollback((s, a, chars) =>
      received.push([s, a, chars]),
    );

    emitAssistantRollback(sessionId, assistantId, 5);
    off();
    emitAssistantRollback(sessionId, assistantId, 9);

    expect(received).toEqual([[sessionId, assistantId, 5]]);
  });
});
