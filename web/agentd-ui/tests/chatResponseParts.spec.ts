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

  it("inserts an approval request at the point it arrives in the stream", () => {
    const state = makeState();

    feed(state, { type: "delta", data: "I need permission to continue." });
    feed(state, {
      type: "input_request",
      request_id: "approval-1",
      run_id: "run-1",
      question: "Allow the command?",
      reason: "The command modifies the project.",
      choices: [{ id: "approve", label: "Approve" }],
      allow_free_text: false,
      multiple: false,
    });

    const messageAfterRequest = state.messagesBySession.value[sessionId][0];
    expect(messageAfterRequest.inputRequests).toEqual([
      expect.objectContaining({
        id: "approval-1",
        question: "Allow the command?",
        status: "pending",
      }),
    ]);
    expect(messageAfterRequest.responseParts).toEqual([
      {
        id: "text-0",
        type: "text",
        content: "I need permission to continue.",
      },
      {
        id: "input-request-approval-1",
        type: "input_request",
        requestId: "approval-1",
      },
    ]);

    feed(state, { type: "delta", data: "Continuing after approval." });

    expect(state.messagesBySession.value[sessionId][0].responseParts).toEqual([
      {
        id: "text-0",
        type: "text",
        content: "I need permission to continue.",
      },
      {
        id: "input-request-approval-1",
        type: "input_request",
        requestId: "approval-1",
      },
      {
        id: "text-2",
        type: "text",
        content: "Continuing after approval.",
      },
    ]);
  });

  it("does not drop an approval event when the assistant placeholder is missing", () => {
    const state = makeState();
    state.setMessages(sessionId, []);

    feed(state, {
      type: "input_request",
      request_id: "approval-without-placeholder",
      run_id: "run-1",
      question: "Approve this tool call?",
      choices: [],
      allow_free_text: true,
      multiple: false,
      created_at: "2026-07-20T00:00:01.000Z",
    });

    expect(state.messagesBySession.value[sessionId]).toEqual([
      expect.objectContaining({
        id: assistantId,
        role: "assistant",
        streaming: true,
        inputRequests: [
          expect.objectContaining({
            id: "approval-without-placeholder",
            status: "pending",
          }),
        ],
        responseParts: [
          {
            id: "input-request-approval-without-placeholder",
            type: "input_request",
            requestId: "approval-without-placeholder",
          },
        ],
      }),
    ]);
  });
});
