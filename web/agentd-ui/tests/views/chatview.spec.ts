import { render, fireEvent, waitFor } from "@testing-library/vue";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import ChatView from "@/views/ChatView.vue";

const chatApiMocks = vi.hoisted(() => ({
  streamAgentRun: vi.fn(async () => {}),
}));

vi.mock("@/api/client", () => ({
  listProjects: async () => [{ id: "proj-1", name: "Demo Project" }],
  listSpecialists: async () => [
    { name: "orchestrator", model: "gpt-5" },
    { name: "orchestrator-max", model: "gpt-5" },
  ],
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
  listChatSessions: async () => [],
  fetchChatMessages: async () => [],
  createChatSession: async () => ({ id: "session-1", name: "Session" }),
  deleteChatSession: async () => {},
  renameChatSession: async () => {},
  generateChatSessionTitle: async () => "Session",
  streamAgentRun: chatApiMocks.streamAgentRun,
  streamAgentVisionRun: vi.fn(async () => {}),
}));

beforeEach(() => {
  chatApiMocks.streamAgentRun.mockClear();
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

// Smoke test for chat send interaction

describe("ChatView", () => {
  it("sends a message and echoes", async () => {
    const { findByPlaceholderText, getByText } = render(ChatView);

    const input = (await findByPlaceholderText(
      "Message the agent...",
    )) as HTMLTextAreaElement;
    await fireEvent.update(input, "hello");
    await fireEvent.submit(input.form as HTMLFormElement);

    expect(getByText("hello")).toBeTruthy();
  });

  it("routes by leading @specialist tag and strips it from provider prompt", async () => {
    const { findByPlaceholderText } = render(ChatView);

    const input = (await findByPlaceholderText(
      "Message the agent...",
    )) as HTMLTextAreaElement;
    await fireEvent.update(input, "@orchestrator-max write a haiku");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.streamAgentRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.streamAgentRun.mock.calls.at(-1)?.[0] as {
      specialist?: string;
      prompt?: string;
    };
    expect(args.specialist).toBe("orchestrator-max");
    expect(args.prompt).toBe("write a haiku");
  });
});
