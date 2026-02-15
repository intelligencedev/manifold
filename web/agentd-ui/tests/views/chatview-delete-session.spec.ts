import { fireEvent, render, waitFor } from "@testing-library/vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ChatView from "@/views/ChatView.vue";

const chatApiMocks = vi.hoisted(() => ({
  deleteChatSession: vi.fn(async () => {}),
}));

vi.mock("@/api/client", () => ({
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
  listChatSessions: async () => [
    {
      id: "session-1",
      name: "Roadmap Chat",
      createdAt: "2026-02-14T10:00:00Z",
      updatedAt: "2026-02-14T10:00:00Z",
      lastMessagePreview: "Hello",
      messageCount: 1,
    },
  ],
  fetchChatMessages: async () => [],
  createChatSession: async () => ({
    id: "session-2",
    name: "New Chat",
    createdAt: "2026-02-14T10:01:00Z",
    updatedAt: "2026-02-14T10:01:00Z",
    messageCount: 0,
  }),
  deleteChatSession: chatApiMocks.deleteChatSession,
  renameChatSession: async (id: string, name: string) => ({
    id,
    name,
    createdAt: "2026-02-14T10:00:00Z",
    updatedAt: "2026-02-14T10:02:00Z",
    messageCount: 1,
  }),
  deleteChatMessage: async () => {},
  deleteChatMessagesAfter: async () => {},
  generateChatSessionTitle: async () => ({
    id: "session-1",
    name: "Roadmap Chat",
    createdAt: "2026-02-14T10:00:00Z",
    updatedAt: "2026-02-14T10:00:00Z",
    messageCount: 1,
  }),
  streamAgentRun: async function* () {},
  streamAgentVisionRun: async function* () {},
}));

describe("ChatView conversation delete", () => {
  beforeEach(() => {
    chatApiMocks.deleteChatSession.mockClear();
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

  it("shows cancel/delete confirmation before deleting a conversation", async () => {
    const { findByRole, getByRole } = render(ChatView);

    const openDeleteDialog = await findByRole("button", {
      name: /Delete conversation Roadmap Chat/i,
    });
    await fireEvent.click(openDeleteDialog);

    const deleteButton = getByRole("button", {
      name: /^Delete Conversation$/i,
    }) as HTMLButtonElement;
    const cancelButton = getByRole("button", {
      name: /^Cancel$/i,
    }) as HTMLButtonElement;

    expect(deleteButton.disabled).toBe(false);

    await fireEvent.click(cancelButton);
    expect(chatApiMocks.deleteChatSession).toHaveBeenCalledTimes(0);

    await fireEvent.click(openDeleteDialog);
    await fireEvent.click(deleteButton);
    await waitFor(() => {
      expect(chatApiMocks.deleteChatSession).toHaveBeenCalledTimes(1);
      expect(chatApiMocks.deleteChatSession).toHaveBeenCalledWith("session-1");
    });
  });
});
