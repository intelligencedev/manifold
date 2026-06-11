import { fireEvent, render, waitFor } from "@testing-library/vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ChatView from "@/views/ChatView.vue";

const chatApiMocks = vi.hoisted(() => ({
  sessions: [
    {
      id: "recent",
      name: "Recent Chat",
      createdAt: "2026-02-14T12:00:00Z",
      updatedAt: "2026-02-14T12:00:00Z",
      lastMessagePreview: "Latest work",
      messageCount: 2,
      projectId: "proj-1",
      pinned: false,
    },
    {
      id: "pinned",
      name: "Pinned Chat",
      createdAt: "2026-02-13T12:00:00Z",
      updatedAt: "2026-02-13T12:00:00Z",
      lastMessagePreview: "Important",
      messageCount: 1,
      projectId: "proj-1",
      pinned: true,
    },
  ],
  updateChatSessionPinned: vi.fn(async (id: string, pinned: boolean) => {
    const session =
      chatApiMocks.sessions.find((candidate) => candidate.id === id) ??
      chatApiMocks.sessions[0];
    return { ...session, pinned };
  }),
}));

vi.mock("@/api/client", () => ({
  fetchAgentdSettings: async () => ({
    openaiSummaryModel: "gpt-5",
    openaiSummaryUrl: "https://api.openai.com/v1",
    summaryEnabled: true,
    summaryContextWindowTokens: 32000,
    summaryPlainTextContextWindowTokens: 90000,
    summaryReserveBufferTokens: 25000,
    summaryMinKeepLastMessages: 4,
    summaryMaxKeepLastMessages: 12,
    summaryMaxSummaryChunkTokens: 4096,
    summaryCallTimeoutSeconds: 120,
    summaryTokenBudget: 7000,
    requestInfoEnabled: true,
  }),
  listProjects: async () => [
    {
      id: "proj-1",
      name: "Demo Project",
      createdAt: "2026-02-14T10:00:00Z",
      updatedAt: "2026-02-14T10:00:00Z",
      sizeBytes: 0,
      files: 0,
    },
  ],
  listSpecialists: async () => [{ name: "orchestrator", model: "gpt-5" }],
  listTeams: async () => [],
  getUserPreferences: async () => ({ activeProjectId: "proj-1" }),
  setActiveProject: async () => {},
  createProject: async () => ({ id: "proj-1", name: "Demo Project" }),
  deleteProject: async () => {},
  listProjectTree: async () => [],
  uploadFile: async () => {},
  deletePath: async () => {},
  createDir: async () => {},
  moveProjectPath: async () => {},
}));

vi.mock("@/api/chat", () => ({
  listChatSessions: async () => chatApiMocks.sessions,
  fetchChatMessages: async () => [],
  fetchChatActivities: async () => [],
  createChatSession: async () => ({
    id: "new-session",
    name: "New Chat",
    createdAt: "2026-02-14T12:05:00Z",
    updatedAt: "2026-02-14T12:05:00Z",
    messageCount: 0,
    projectId: "proj-1",
    pinned: false,
  }),
  deleteChatSession: async () => {},
  renameChatSession: async (id: string, name: string) => ({
    id,
    name,
    createdAt: "2026-02-14T12:00:00Z",
    updatedAt: "2026-02-14T12:00:00Z",
    messageCount: 2,
    projectId: "proj-1",
    pinned: id === "pinned",
  }),
  updateChatSessionProject: async (id: string, projectId: string) => ({
    id,
    name: id === "pinned" ? "Pinned Chat" : "Recent Chat",
    projectId,
  }),
  updateChatSessionMemorySettings: async (
    id: string,
    settings: { memoryEnabled?: boolean },
  ) => ({
    id,
    memoryEnabled: settings.memoryEnabled ?? true,
    evolvingMemoryEnabled: settings.memoryEnabled ?? true,
    beliefMemoryEnabled: settings.memoryEnabled ?? true,
  }),
  updateChatSessionCommandPolicyAllowAll: async (
    id: string,
    allow: boolean,
  ) => ({
    id,
    commandPolicyAllowAll: allow,
  }),
  updateChatSessionPinned: chatApiMocks.updateChatSessionPinned,
  listActiveChatRuns: async () => [],
  deleteChatMessage: async () => {},
  deleteChatMessagesAfter: async () => {},
  generateChatSessionTitle: async () => chatApiMocks.sessions[0],
  streamAgentRun: async function* () {},
  streamAgentVisionRun: async function* () {},
}));

function appearsBefore(first: Element, second: Element) {
  return Boolean(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
  );
}

function conversationRowFor(button: HTMLElement) {
  const row = button.closest(".conversation-session-row");
  if (!(row instanceof HTMLElement)) {
    throw new Error("Expected conversation row");
  }
  return row;
}

describe("ChatView conversation pinning", () => {
  beforeEach(() => {
    chatApiMocks.updateChatSessionPinned.mockClear();
    vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
      if (String(input).includes("/api/me")) {
        return new Response(JSON.stringify({ name: "Test User" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("", { status: 204 });
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps pinned conversations above regular conversations", async () => {
    const { findByRole, getByRole } = render(ChatView);

    const pinnedButton = await findByRole("button", {
      name: /Unpin conversation Pinned Chat/i,
    });
    const recentButton = await findByRole("button", {
      name: /Pin conversation Recent Chat/i,
    });
    expect(
      appearsBefore(
        conversationRowFor(pinnedButton),
        conversationRowFor(recentButton),
      ),
    ).toBe(true);

    await fireEvent.click(recentButton);

    await waitFor(() => {
      expect(chatApiMocks.updateChatSessionPinned).toHaveBeenCalledWith(
        "recent",
        true,
      );
    });
    await waitFor(() => {
      expect(
        appearsBefore(
          conversationRowFor(
            getByRole("button", {
              name: /Unpin conversation Recent Chat/i,
            }),
          ),
          conversationRowFor(
            getByRole("button", {
              name: /Unpin conversation Pinned Chat/i,
            }),
          ),
        ),
      ).toBe(true);
    });
  });
});
