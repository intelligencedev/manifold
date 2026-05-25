import { render, fireEvent, waitFor } from "@testing-library/vue";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import ChatView from "@/views/ChatView.vue";
import type { StreamAgentRunOptions } from "@/api/chat";

const chatApiMocks = vi.hoisted(() => ({
  streamAgentRun: vi.fn(async (_options: StreamAgentRunOptions) => {}),
  updateChatSessionMemorySettings: vi.fn(
    async (
      id: string,
      settings: {
        evolvingMemoryEnabled?: boolean;
        beliefMemoryEnabled?: boolean;
      },
    ) => ({
      id,
      name: "Session",
      projectId: "proj-1",
      evolvingMemoryEnabled: settings.evolvingMemoryEnabled ?? true,
      beliefMemoryEnabled: settings.beliefMemoryEnabled ?? true,
    }),
  ),
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
  fetchChatActivities: async () => [],
  createChatSession: async () => ({
    id: "session-1",
    name: "Session",
    projectId: "proj-1",
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  deleteChatSession: async () => {},
  renameChatSession: async () => {},
  updateChatSessionProject: async (id: string, projectId: string) => ({
    id,
    name: "Session",
    projectId,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  updateChatSessionMemorySettings:
    chatApiMocks.updateChatSessionMemorySettings,
  generateChatSessionTitle: async () => ({
    id: "session-1",
    name: "Session",
    projectId: "proj-1",
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  streamAgentRun: chatApiMocks.streamAgentRun,
  streamAgentVisionRun: vi.fn(async () => {}),
}));

beforeEach(() => {
  chatApiMocks.streamAgentRun.mockClear();
  chatApiMocks.updateChatSessionMemorySettings.mockClear();
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
    const { findByLabelText, findByPlaceholderText, getByText } =
      render(ChatView);

    const input = (await findByPlaceholderText(
      "Message the agent...",
    )) as HTMLTextAreaElement;
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });
    await fireEvent.update(input, "hello");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(getByText("hello")).toBeTruthy();
    });
  });

  it("routes by leading @specialist tag and strips it from provider prompt", async () => {
    const { findByLabelText, findByPlaceholderText } = render(ChatView);

    const input = (await findByPlaceholderText(
      "Message the agent...",
    )) as HTMLTextAreaElement;
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });
    await fireEvent.update(input, "@orchestrator-max write a haiku");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.streamAgentRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.streamAgentRun.mock.calls.at(-1)?.[0];
    expect(args?.specialist).toBe("orchestrator-max");
    expect(args?.projectId).toBe("proj-1");
    expect(args?.prompt).toBe("write a haiku");
  });

  it("sends the active session memory toggles with a run", async () => {
    const { findByLabelText, findByPlaceholderText } = render(ChatView);

    const input = (await findByPlaceholderText(
      "Message the agent...",
    )) as HTMLTextAreaElement;
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });

    const memoryToggle = (await findByLabelText(
      "Evolving memory",
    )) as HTMLInputElement;
    await fireEvent.click(memoryToggle);
    await waitFor(() => {
      expect(chatApiMocks.updateChatSessionMemorySettings).toHaveBeenCalledWith(
        "session-1",
        { evolvingMemoryEnabled: false, beliefMemoryEnabled: undefined },
      );
    });

    await fireEvent.update(input, "remember less");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.streamAgentRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.streamAgentRun.mock.calls.at(-1)?.[0];
    expect(args?.evolvingMemoryEnabled).toBe(false);
    expect(args?.beliefMemoryEnabled).toBe(true);
  });
});
