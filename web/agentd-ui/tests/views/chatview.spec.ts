import { render, fireEvent, waitFor } from "@testing-library/vue";
import { QueryClient, VueQueryPlugin } from "@tanstack/vue-query";
import { createPinia } from "pinia";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import ChatView from "@/views/ChatView.vue";
import type { ChatStreamEvent, StreamAgentRunOptions } from "@/api/chat";

const chatApiMocks = vi.hoisted(() => ({
  sessions: [] as Array<Record<string, unknown>>,
  messages: [] as Array<Record<string, unknown>>,
  activities: [] as Array<Record<string, unknown>>,
  specialists: [
    { name: "orchestrator", model: "gpt-5", paused: false },
    { name: "orchestrator-max", model: "gpt-5", paused: false },
    { name: "ops", model: "gpt-specialist", paused: false },
  ],
  teams: [
    {
      name: "ops",
      orchestratorName: "orchestrator-max",
      members: ["orchestrator-max", "ops"],
    },
  ],
  listProjects: vi.fn(async () => [{ id: "proj-1", name: "Demo Project" }]),
  startChatRun: vi.fn(async (_options: StreamAgentRunOptions) => ({
    run_id: "run-1",
    session_id: "session-1",
    user_message_id: "user-1",
    assistant_message_id: "assistant-1",
    status: "running",
  })),
  streamChatRunEvents: vi.fn(
    async (options: { onEvent: (event: ChatStreamEvent) => void }) => {
      options.onEvent({ type: "final", data: "done", sequence: 1 });
    },
  ),
  updateChatSessionMemorySettings: vi.fn(
    async (
      id: string,
      settings: {
        memoryEnabled?: boolean;
        evolvingMemoryEnabled?: boolean;
        beliefMemoryEnabled?: boolean;
      },
    ) => ({
      id,
      name: "Session",
      projectId: "proj-1",
      memoryEnabled: settings.memoryEnabled ?? true,
      evolvingMemoryEnabled: settings.memoryEnabled ?? true,
      beliefMemoryEnabled: settings.memoryEnabled ?? true,
    }),
  ),
  updateChatSessionPinned: vi.fn(async (id: string, pinned: boolean) => ({
    id,
    name: "Session",
    projectId: "proj-1",
    pinned,
    memoryEnabled: true,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  })),
  updateChatSessionActiveTarget: vi.fn(
    async (
      id: string,
      target: { activeSpecialist: string; activeTeam: string },
    ) => ({
      id,
      name: "Session",
      projectId: "proj-1",
      activeSpecialist: target.activeSpecialist,
      activeTeam: target.activeTeam,
      memoryEnabled: true,
      evolvingMemoryEnabled: true,
      beliefMemoryEnabled: true,
    }),
  ),
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
  listProjects: chatApiMocks.listProjects,
  listSpecialists: async () => chatApiMocks.specialists,
  listTeams: async () => chatApiMocks.teams,
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
  fetchChatMessages: async () => chatApiMocks.messages,
  fetchChatActivities: async () => chatApiMocks.activities,
  createChatSession: async () => ({
    id: "session-1",
    name: "Session",
    projectId: "proj-1",
    memoryEnabled: true,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  deleteChatSession: async () => {},
  renameChatSession: async () => {},
  updateChatSessionProject: async (id: string, projectId: string) => ({
    id,
    name: "Session",
    projectId,
    memoryEnabled: true,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  updateChatSessionMemorySettings: chatApiMocks.updateChatSessionMemorySettings,
  updateChatSessionCommandPolicyAllowAll: async (
    id: string,
    allow: boolean,
  ) => ({
    id,
    name: "Session",
    projectId: "proj-1",
    commandPolicyAllowAll: allow,
    memoryEnabled: true,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  updateChatSessionActiveTarget: chatApiMocks.updateChatSessionActiveTarget,
  updateChatSessionPinned: chatApiMocks.updateChatSessionPinned,
  generateChatSessionTitle: async () => ({
    id: "session-1",
    name: "Session",
    projectId: "proj-1",
    memoryEnabled: true,
    evolvingMemoryEnabled: true,
    beliefMemoryEnabled: true,
  }),
  listActiveChatRuns: async () => [],
  resumeChatRun: vi.fn(async () => {}),
  startChatRun: chatApiMocks.startChatRun,
  streamChatRunEvents: chatApiMocks.streamChatRunEvents,
  streamAgentVisionRun: vi.fn(async () => {}),
}));

beforeEach(() => {
  chatApiMocks.sessions = [];
  chatApiMocks.messages = [];
  chatApiMocks.activities = [];
  chatApiMocks.specialists = [
    { name: "orchestrator", model: "gpt-5", paused: false },
    { name: "orchestrator-max", model: "gpt-5", paused: false },
    { name: "ops", model: "gpt-specialist", paused: false },
  ];
  chatApiMocks.teams = [
    {
      name: "ops",
      orchestratorName: "orchestrator-max",
      members: ["orchestrator-max", "ops"],
    },
  ];
  chatApiMocks.startChatRun.mockClear();
  chatApiMocks.streamChatRunEvents.mockClear();
  chatApiMocks.listProjects.mockClear();
  chatApiMocks.updateChatSessionMemorySettings.mockClear();
  chatApiMocks.updateChatSessionActiveTarget.mockClear();
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

// Smoke test for chat send interaction

describe("ChatView", () => {
  function renderChatView() {
    return render(ChatView, {
      global: {
        plugins: [
          createPinia(),
          [VueQueryPlugin, { queryClient: new QueryClient() }],
        ],
      },
    });
  }

  function sessionMeta(overrides: Record<string, unknown> = {}) {
    return {
      id: "session-1",
      name: "Session",
      createdAt: "2026-01-01T00:00:00.000Z",
      updatedAt: "2026-01-01T00:00:00.000Z",
      projectId: "proj-1",
      memoryEnabled: true,
      evolvingMemoryEnabled: true,
      beliefMemoryEnabled: true,
      ...overrides,
    };
  }

  function configureOpsTeam() {
    chatApiMocks.teams = [
      {
        name: "ops",
        orchestratorName: "orchestrator-max",
        members: ["orchestrator-max", "ops"],
      },
    ];
  }

  it("loads projects without expensive usage stats on mount", async () => {
    renderChatView();

    await waitFor(() => {
      expect(chatApiMocks.listProjects).toHaveBeenCalledWith({
        includeUsage: false,
      });
    });
  });

  it("renders the redesigned chat landmarks with real application labels", async () => {
    const { findByRole, getByText } = renderChatView();

    await findByRole("heading", { name: "Conversations" });

    expect(getByText("Workspace")).toBeTruthy();
    expect(getByText("Active conversation")).toBeTruthy();
    expect(getByText("Participants")).toBeTruthy();
    expect(getByText("Team routing")).toBeTruthy();
    expect(getByText("Run Activity")).toBeTruthy();
    expect(getByText("Execution Timeline")).toBeTruthy();
    expect(getByText("Tool Invocations")).toBeTruthy();
    expect(getByText("Model & Performance")).toBeTruthy();
    expect(getByText("Context & Memory")).toBeTruthy();
  });

  it("renders execution activity as a time grid and tool invocation table", async () => {
    chatApiMocks.sessions = [sessionMeta()];
    chatApiMocks.messages = [
      {
        id: "assistant-1",
        role: "assistant",
        content: "Coordinating specialist work.",
        createdAt: "2026-01-01T00:00:00.000Z",
      },
    ];
    chatApiMocks.activities = [
      {
        callId: "call-1",
        assistantMessageId: "assistant-1",
        agent: "Topology Expert",
        model: "gpt-4o",
        prompt: "Compute invariants",
        depth: 1,
        status: "done",
        content: "Computed invariants.",
        entries: [
          {
            id: "tool-1",
            type: "tool",
            title: "web_search",
            content: "Search academic papers on Klein bottle curvature",
            createdAt: "2026-01-01T00:00:01.000Z",
          },
        ],
        thoughtSummaries: [],
        startedAt: "2026-01-01T00:00:01.000Z",
        finishedAt: "2026-01-01T00:00:03.000Z",
      },
      {
        callId: "call-2",
        assistantMessageId: "assistant-1",
        agent: "Geometry Analyst",
        model: "claude-3.5-sonnet",
        prompt: "Compare manifolds",
        depth: 1,
        status: "running",
        content: "",
        entries: [],
        thoughtSummaries: [],
        startedAt: "2026-01-01T00:00:08.000Z",
      },
    ];

    const { findByRole, findAllByText, getAllByText, getByText } =
      renderChatView();

    await findByRole("heading", { name: "Conversations" });
    await findAllByText("Topology Expert");

    expect(getByText("00:00")).toBeTruthy();
    expect(getByText("00:05")).toBeTruthy();
    expect(getAllByText("Geometry Analyst").length).toBeGreaterThan(0);
    expect(getAllByText("web_search").length).toBeGreaterThan(0);
    expect(getByText("Duration")).toBeTruthy();
    expect(getAllByText("2.0s").length).toBeGreaterThan(0);
    expect(getByText("Timestamp")).toBeTruthy();
    expect(getByText("00:00:01")).toBeTruthy();
  });

  async function waitForProjectSelection(
    findByLabelText: (text: string) => Promise<HTMLElement>,
  ) {
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });
  }

  async function waitForTeamOption(
    findByLabelText: (text: string) => Promise<HTMLElement>,
    value: string,
  ) {
    const teamSelect = (await findByLabelText(
      "Specialist team",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(
        Array.from(teamSelect.options).some((option) => option.value === value),
      ).toBe(true);
    });
    return teamSelect;
  }

  async function findComposer() {
    await waitFor(() => {
      expect(document.querySelector("textarea")).toBeTruthy();
    });
    return document.querySelector("textarea") as HTMLTextAreaElement;
  }

  it("shows the active orchestrator in the prompt placeholder", async () => {
    const { findByPlaceholderText } = renderChatView();

    await findByPlaceholderText("Message orchestrator...");
  });

  it("sends a message and echoes", async () => {
    const { findByLabelText, getByText } = renderChatView();

    const input = await findComposer();
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
    const { findByLabelText } = renderChatView();

    const input = await findComposer();
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });
    await fireEvent.update(input, "@orchestrator-max write a haiku");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.specialist).toBe("orchestrator-max");
    expect(args?.projectId).toBe("proj-1");
    expect(args?.prompt).toBe("write a haiku");
    await waitFor(() => {
      expect(chatApiMocks.updateChatSessionActiveTarget).toHaveBeenCalledWith(
        "session-1",
        {
          activeSpecialist: "orchestrator-max",
          activeTeam: "",
        },
      );
    });
  });

  it("routes by leading @team tag and strips it from provider prompt", async () => {
    configureOpsTeam();
    const { findByLabelText } = renderChatView();

    const input = await findComposer();
    await waitForProjectSelection(findByLabelText);
    await waitForTeamOption(findByLabelText, "ops");
    await fireEvent.update(input, "@ops write a rollout plan");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.teamName).toBe("ops");
    expect(args?.specialist).toBeUndefined();
    expect(args?.prompt).toBe("write a rollout plan");
  });

  it("prefers a team when a leading mention matches a team and specialist", async () => {
    configureOpsTeam();
    chatApiMocks.specialists = [
      ...chatApiMocks.specialists,
      { name: "ops", model: "gpt-specialist", paused: false },
    ];
    const { findByLabelText } = renderChatView();

    const input = await findComposer();
    await waitForProjectSelection(findByLabelText);
    await waitForTeamOption(findByLabelText, "ops");
    await fireEvent.update(input, "@ops inspect the incident");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.teamName).toBe("ops");
    expect(args?.specialist).toBeUndefined();
    expect(args?.prompt).toBe("inspect the incident");
  });

  it("routes untagged prompts through selected team and returns to default on all participants", async () => {
    configureOpsTeam();
    const { findByLabelText } = renderChatView();

    const input = await findComposer();
    await waitForProjectSelection(findByLabelText);
    const teamSelect = await waitForTeamOption(findByLabelText, "ops");

    await fireEvent.update(teamSelect, "ops");
    await fireEvent.update(input, "status update");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalled();
    });
    let args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.teamName).toBe("ops");
    expect(args?.specialist).toBeUndefined();
    await waitFor(() => {
      expect(chatApiMocks.updateChatSessionActiveTarget).toHaveBeenCalledWith(
        "session-1",
        {
          activeSpecialist: "orchestrator",
          activeTeam: "ops",
        },
      );
    });

    await fireEvent.update(teamSelect, "");
    await fireEvent.update(input, "default status");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalledTimes(2);
    });
    args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.teamName).toBeUndefined();
    expect(args?.specialist).toBeUndefined();
  });

  it("shows the selected team orchestrator instead of the default orchestrator participant", async () => {
    configureOpsTeam();
    const { findByLabelText } = renderChatView();

    await waitForProjectSelection(findByLabelText);
    const teamSelect = await waitForTeamOption(findByLabelText, "ops");
    await fireEvent.update(teamSelect, "ops");

    await waitFor(() => {
      expect(document.querySelector("textarea")?.placeholder).toBe(
        "Message orchestrator-max...",
      );
    });
    await waitFor(() => {
      const participantNames = Array.from(
        document.querySelectorAll(".participant-name"),
      ).map((el) => el.textContent?.trim());
      expect(participantNames).toContain("orchestrator-max");
      expect(participantNames).toContain("ops");
      expect(participantNames).not.toContain("orchestrator");
    });
  });

  it("sends the active session memory toggles with a run", async () => {
    const { findByLabelText } = renderChatView();

    const input = await findComposer();
    const projectSelect = (await findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(projectSelect.value).toBe("proj-1");
    });

    const memoryToggle = (await findByLabelText("Memory")) as HTMLInputElement;
    await fireEvent.click(memoryToggle);
    await waitFor(() => {
      expect(chatApiMocks.updateChatSessionMemorySettings).toHaveBeenCalledWith(
        "session-1",
        { memoryEnabled: false },
      );
    });

    await fireEvent.update(input, "remember less");
    await fireEvent.submit(input.form as HTMLFormElement);

    await waitFor(() => {
      expect(chatApiMocks.startChatRun).toHaveBeenCalled();
    });
    const args = chatApiMocks.startChatRun.mock.calls.at(-1)?.[0];
    expect(args?.memoryEnabled).toBe(false);
  });

  it("hydrates the prompt placeholder from the persisted active specialist", async () => {
    chatApiMocks.sessions = [sessionMeta({ activeSpecialist: "ops" })];
    const { findByPlaceholderText } = renderChatView();

    await findByPlaceholderText("Message ops...");
    expect(chatApiMocks.updateChatSessionActiveTarget).not.toHaveBeenCalled();
  });
});
