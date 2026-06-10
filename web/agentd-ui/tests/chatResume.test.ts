import { describe, expect, it, vi, beforeEach } from "vitest";
import { createChatStoreState } from "@/stores/chatStoreState";
import { createChatStreamActions } from "@/stores/chatStreamActions";
import type { ChatStreamEvent } from "@/api/chat";

const chatApiMocks = vi.hoisted(() => ({
  resumeChatRun: vi.fn(),
  streamChatRunEvents: vi.fn(),
}));

vi.mock("@/api/chat", () => ({
  cancelChatRun: vi.fn(async () => {}),
  generateChatSessionTitle: vi.fn(async () => ({
    id: "session-1",
    name: "Session",
  })),
  resumeChatRun: chatApiMocks.resumeChatRun,
  streamAgentRun: vi.fn(async () => {}),
  streamAgentVisionRun: vi.fn(async () => {}),
  streamChatRunEvents: chatApiMocks.streamChatRunEvents,
}));

describe("chat durable resume", () => {
  beforeEach(() => {
    chatApiMocks.resumeChatRun.mockReset();
    chatApiMocks.streamChatRunEvents.mockReset();
  });

  it("resumes after the retry boundary and clears stale failed state", async () => {
    const state = createChatStoreState();
    state.sessions.value = [
      {
        id: "session-1",
        name: "Session",
        createdAt: "2026-06-02T12:00:00.000Z",
        updatedAt: "2026-06-02T12:00:00.000Z",
      },
    ];
    state.activeSessionId.value = "session-1";
    state.setMessages("session-1", [
      {
        id: "assistant-1",
        role: "assistant",
        content: "old work",
        createdAt: "2026-06-02T12:00:00.000Z",
        runId: "run-1",
        lastRunSequence: 7,
        error: "chatStream SSE fallback: status 500",
      },
    ]);
    chatApiMocks.resumeChatRun.mockResolvedValue({
      run_id: "run-1",
      status: "queued",
      retried: true,
      last_sequence: 8,
      last_retry_sequence: 8,
    });
    chatApiMocks.streamChatRunEvents.mockImplementation(
      async (options: {
        after?: number;
        onEvent: (event: ChatStreamEvent) => void;
      }) => {
        options.onEvent({ type: "delta", data: " resumed", sequence: 9 });
        options.onEvent({
          type: "final",
          data: "old work resumed",
          sequence: 10,
          durationMs: 1234,
        });
      },
    );

    const actions = createChatStreamActions(
      state,
      { invalidateQueries: vi.fn() },
      {} as any,
    );
    await actions.resumeDurableRun("session-1", "assistant-1", "run-1");

    expect(chatApiMocks.resumeChatRun).toHaveBeenCalledWith("run-1");
    expect(chatApiMocks.streamChatRunEvents).toHaveBeenCalledWith(
      expect.objectContaining({ runId: "run-1", after: 8 }),
    );
    const [message] = state.messagesBySession.value["session-1"];
    expect(message.streaming).toBe(false);
    expect(message.error).toBeUndefined();
    expect(message.content).toBe("old work resumed");
    expect(message.durationMs).toBe(1234);
    expect(message.lastRunSequence).toBe(10);
  });
});
