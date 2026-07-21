import { describe, expect, it } from "vitest";
import { createChatStoreState } from "@/stores/chatStoreState";
import { handleStreamEvent } from "@/stores/chatStreamEvents";

const sessionId = "session-1";
const assistantId = "assistant-1";
const streamId = "stream-1";

function makeState(content = "") {
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
      content,
      createdAt: "2026-07-20T00:00:00.000Z",
      streaming: true,
    },
  ]);
  return state;
}

function feed(state: ReturnType<typeof createChatStoreState>, event: object) {
  handleStreamEvent(
    state,
    { invalidateQueries: () => undefined },
    event as never,
    sessionId,
    assistantId,
    streamId,
  );
}

describe("streamed response parts", () => {
  it("keeps tool calls between the text emitted before and after them", () => {
    const state = makeState();

    feed(state, { type: "delta", data: "I’ll inspect the repository." });
    feed(state, {
      type: "tool_start",
      tool_id: "call-1",
      tool_name: "search",
      tool_title: "Search repository",
      args: '{"query":"activity"}',
    });
    feed(state, {
      type: "tool_result",
      tool_id: "call-1",
      tool_name: "search",
      tool_title: "Search repository",
      data: "3 matches",
    });
    feed(state, { type: "delta", data: "I found three relevant matches." });

    expect(state.messagesBySession.value[sessionId][0].responseParts).toEqual([
      {
        id: "text-0",
        type: "text",
        content: "I’ll inspect the repository.",
      },
      {
        id: "tool-call-1",
        type: "tool",
        title: "Search repository",
        status: "done",
        args: '{"query":"activity"}',
        result: "3 matches",
      },
      {
        id: "text-2",
        type: "text",
        content: "I found three relevant matches.",
      },
    ]);
  });

  it("seeds ordered parts from existing response text when a run resumes", () => {
    const state = makeState("Existing response. ");

    feed(state, {
      type: "tool_start",
      tool_id: "call-2",
      tool_name: "read",
      tool_title: "Read file",
    });
    feed(state, { type: "delta", data: "Resumed response." });

    expect(state.messagesBySession.value[sessionId][0].responseParts).toEqual([
      {
        id: `${assistantId}-text`,
        type: "text",
        content: "Existing response. ",
      },
      {
        id: "tool-call-2",
        type: "tool",
        title: "Read file",
        status: "running",
        args: undefined,
      },
      {
        id: "text-2",
        type: "text",
        content: "Resumed response.",
      },
    ]);
  });
});
