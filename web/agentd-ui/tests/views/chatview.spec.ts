import { render, fireEvent } from "@testing-library/vue";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import ChatView from "@/views/ChatView.vue";

vi.mock("@/api/client", () => ({
  listProjects: async () => [{ id: "proj-1", name: "Demo Project" }],
  listSpecialists: async () => [{ name: "orchestrator", model: "gpt-5" }],
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
  streamAgentRun: async function* () {},
  streamAgentVisionRun: async function* () {},
}));

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL) => {
      if (String(input).includes("/api/me")) {
        return new Response(JSON.stringify({ name: "Test User" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("", { status: 204 });
    },
  );
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
});
