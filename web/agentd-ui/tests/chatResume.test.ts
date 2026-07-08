import { describe, expect, it, vi, beforeEach } from "vitest";
import { createChatStoreState } from "@/stores/chatStoreState";
import { createChatStreamActions } from "@/stores/chatStreamActions";
import type { ChatStreamEvent } from "@/api/chat";

const chatApiMocks = vi.hoisted(() => ({
  resumeChatRun: vi.fn(),
  startChatRun: vi.fn(),
  streamChatRunEvents: vi.fn(),
}));

vi.mock("@/api/chat", () => ({
  cancelChatRun: vi.fn(async () => {}),
  generateChatSessionTitle: vi.fn(async () => ({
    id: "session-1",
    name: "Session",
  })),
  resumeChatRun: chatApiMocks.resumeChatRun,
  startChatRun: chatApiMocks.startChatRun,
  streamAgentVisionRun: vi.fn(async () => {}),
  streamChatRunEvents: chatApiMocks.streamChatRunEvents,
}));

describe("chat durable resume", () => {
  beforeEach(() => {
    chatApiMocks.resumeChatRun.mockReset();
    chatApiMocks.startChatRun.mockReset();
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

  it("reconnects a started run after the event stream drops", async () => {
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
    chatApiMocks.startChatRun.mockResolvedValue({
      run_id: "run-1",
      session_id: "session-1",
      user_message_id: "user-1",
      assistant_message_id: "assistant-1",
      status: "running",
    });
    chatApiMocks.resumeChatRun.mockResolvedValue({
      run_id: "run-1",
      status: "running",
      retried: false,
    });
    chatApiMocks.streamChatRunEvents
      .mockImplementationOnce(
        async (options: {
          after?: number;
          onEvent: (event: ChatStreamEvent) => void;
        }) => {
          expect(options.after).toBe(0);
          options.onEvent({ type: "delta", data: "partial", sequence: 1 });
          throw new TypeError("network error");
        },
      )
      .mockImplementationOnce(
        async (options: {
          after?: number;
          onEvent: (event: ChatStreamEvent) => void;
        }) => {
          expect(options.after).toBe(1);
          options.onEvent({ type: "delta", data: " done", sequence: 2 });
          options.onEvent({
            type: "final",
            data: "partial done",
            sequence: 3,
          });
        },
      );

    const actions = createChatStreamActions(
      state,
      { invalidateQueries: vi.fn() },
      {} as any,
    );
    const started = actions.sendPrompt("hello");
    await new Promise((resolve) => window.setTimeout(resolve, 550));
    await started;

    expect(chatApiMocks.resumeChatRun).toHaveBeenCalledWith("run-1");
    expect(chatApiMocks.streamChatRunEvents).toHaveBeenCalledTimes(2);
    const assistant = state.messagesBySession.value["session-1"].find(
      (message) => message.role === "assistant",
    );
    expect(assistant?.streaming).toBe(false);
    expect(assistant?.error).toBeUndefined();
    expect(assistant?.content).toBe("partial done");
    expect(assistant?.lastRunSequence).toBe(3);
  });

  it("reloads completed chat messages so backend-enriched fields are visible without refresh", async () => {
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
    chatApiMocks.startChatRun.mockResolvedValue({
      run_id: "run-1",
      session_id: "session-1",
      user_message_id: "user-1",
      assistant_message_id: "assistant-1",
      status: "running",
    });
    chatApiMocks.streamChatRunEvents.mockImplementation(
      async (options: { onEvent: (event: ChatStreamEvent) => void }) => {
        options.onEvent({
          type: "final",
          data: "done",
          sequence: 1,
        });
      },
    );
    const loadMessagesFromServer = vi.fn(async (sessionId: string) => {
      state.setMessages(sessionId, [
        {
          id: "user-1",
          role: "user",
          content: "hello",
          createdAt: "2026-06-02T12:00:00.000Z",
        },
        {
          id: "assistant-1",
          role: "assistant",
          content: "done",
          createdAt: "2026-06-02T12:00:01.000Z",
          llmRequestCount: 1,
        },
      ]);
    });

    const actions = createChatStreamActions(
      state,
      { invalidateQueries: vi.fn() },
      { loadMessagesFromServer } as any,
    );
    await actions.sendPrompt("hello");

    expect(loadMessagesFromServer).toHaveBeenCalledWith("session-1", {
      force: true,
    });
    const assistant = state.messagesBySession.value["session-1"].find(
      (message) => message.role === "assistant",
    );
    expect(assistant?.llmRequestCount).toBe(1);
  });
});
