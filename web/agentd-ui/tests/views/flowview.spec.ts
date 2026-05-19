import { fireEvent, render, screen, waitFor } from "@testing-library/vue";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

import FlowView from "@/views/FlowView.vue";

const toolsResponse = [
  {
    name: "web-search",
    description: "Search the web",
    parameters: {
      type: "object",
      properties: {
        query: { type: "string", description: "Search query" },
        limit: { type: "integer", minimum: 1 },
      },
      required: ["query"],
    },
  },
  {
    name: "utility_textbox",
    description: "Utility textbox node",
    parameters: {
      type: "object",
      properties: {
        label: { type: "string" },
        text: { type: "string" },
        output_attr: { type: "string" },
      },
    },
  },
];

const workflowsResponse = [
  {
    id: "default",
    name: "default",
    description: "Sample workflow",
    trigger: { type: "manual" },
    nodes: [
      {
        id: "step-1",
        name: "Start",
        kind: "action",
        type: "tool",
        tool: "web-search",
        inputs: {
          query: { literal: "hello" },
          limit: { literal: 3 },
        },
        publish_result: true,
      },
      {
        id: "utility-1",
        name: "Notes",
        kind: "data",
        type: "tool",
        tool: "utility_textbox",
        inputs: {
          label: { literal: "Notes" },
          text: { literal: "Initial note" },
          output_attr: { literal: "notes_attr" },
        },
      },
    ],
    edges: [],
  },
  {
    id: "saved-workflow",
    name: "saved-workflow",
    description: "Saved workflow",
    trigger: { type: "manual" },
    project_id: "proj-2",
    nodes: [],
    edges: [],
  },
];

vi.mock("@/api/client", () => ({
  listProjects: async () => [
    {
      id: "proj-1",
      name: "Project One",
      createdAt: "2026-02-14T10:00:00Z",
      updatedAt: "2026-02-14T10:00:00Z",
      sizeBytes: 0,
      files: 0,
    },
    {
      id: "proj-2",
      name: "Project Two",
      createdAt: "2026-02-14T10:00:00Z",
      updatedAt: "2026-02-14T10:00:00Z",
      sizeBytes: 0,
      files: 0,
    },
  ],
  getUserPreferences: async () => ({ activeProjectId: "" }),
  setActiveProject: async () => {},
  createProject: async () => ({ id: "proj-1", name: "Project One" }),
  deleteProject: async () => {},
  listProjectTree: async () => [],
  uploadFile: async () => {},
  deletePath: async () => {},
  createDir: async () => {},
  moveProjectPath: async () => {},
  fetchProjectFileText: async () => "",
  saveProjectFileText: async () => {},
}));

beforeEach(() => {
  localStorage.clear();
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.href
            : input.url;

      if (url.endsWith("/api/flows/v2/tools")) {
        return new Response(JSON.stringify(toolsResponse), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }

      if (url.endsWith("/api/flows/v2/workflows")) {
        return new Response(
          JSON.stringify({
            workflows: workflowsResponse.map((workflow) => ({
              id: workflow.id,
              name: workflow.name,
              description: workflow.description,
            })),
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      if (url.includes("/api/flows/v2/workflows/")) {
        const id = decodeURIComponent(url.split("/").pop() || "");
        const workflow =
          workflowsResponse.find((item) => item.id === id) ??
          workflowsResponse[0];
        if (!init || !init.method || init.method === "GET") {
          return new Response(JSON.stringify({ workflow }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (init && init.method === "PUT") {
          try {
            const body = init.body
              ? JSON.parse(init.body as string)
              : { workflow };
            return new Response(JSON.stringify(body), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            });
          } catch {
            return new Response("bad request", { status: 400 });
          }
        }
      }

      if (url.endsWith("/api/flows/v2/run")) {
        return new Response(
          JSON.stringify({ run_id: "run-1", status: "running" }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      if (url.endsWith("/api/flows/v2/runs/run-1/events")) {
        return new Response(
          JSON.stringify({
            run_id: "run-1",
            status: "completed",
            events: [
              {
                run_id: "run-1",
                type: "node_completed",
                node_id: "step-1",
                message: "Start",
                output: {
                  inputs: { query: "hello", limit: 3 },
                },
              },
              {
                run_id: "run-1",
                type: "node_completed",
                node_id: "utility-1",
                message: "Notes",
                output: {
                  result: "ok",
                  inputs: {
                    text: "Initial note",
                    output_attr: "notes_attr",
                  },
                },
              },
            ],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      return new Response("not found", { status: 404 });
    },
  );

  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("FlowView", () => {
  it("shows tool palette and renders utility node content", async () => {
    const { findByText, queryByText } = render(FlowView);

    expect(await findByText("Tool Palette")).toBeTruthy();
    expect(await findByText("Workflow Tools")).toBeTruthy();
    expect(await findByText("web-search")).toBeTruthy();
    expect(await findByText("Utility Nodes")).toBeTruthy();
    expect(await findByText("Textbox")).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Design" })).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Run" })).toBeTruthy();

    expect(await screen.findByDisplayValue("Notes")).toBeTruthy();
    expect(await screen.findByDisplayValue("Initial note")).toBeTruthy();

    expect(queryByText(/Select a node to edit step details/)).toBeNull();
  });

  it("enables Run button and posts to /api/flows/v2/run when clicked", async () => {
    render(FlowView);

    // Wait for workflows to load and Run button to be present
    const runBtn = await screen.findByRole("button", {
      name: "Run workflow",
    });
    expect(runBtn).toBeTruthy();
    await waitFor(() => {
      expect((runBtn as HTMLButtonElement).disabled).toBe(false);
    });

    // Click Run
    await fireEvent.click(runBtn);

    // Expect that a POST to /api/flows/v2/run eventually occurs
    await waitFor(() => {
      const calls = vi.mocked(global.fetch).mock.calls as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      expect(
        calls.some(
          ([u, init]) =>
            String(u).endsWith("/api/flows/v2/run") &&
            (init?.method ?? "GET") === "POST",
        ),
      ).toBe(true);
    });
  });

  it("restores the cached workflow and project selection", async () => {
    localStorage.setItem(
      "flow.selection.v1",
      JSON.stringify({ workflowIntent: "default", projectId: "proj-1" }),
    );

    render(FlowView);

    const workflowSelect = (await screen.findByLabelText(
      "Workflow",
    )) as HTMLSelectElement;
    const projectSelect = (await screen.findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(workflowSelect.value).toBe("default");
      expect(projectSelect.value).toBe("proj-1");
    });

    const saveBtn = await screen.findByRole("button", {
      name: "Save workflow",
    });
    await waitFor(() => {
      expect((saveBtn as HTMLButtonElement).disabled).toBe(false);
    });
  });

  it("prefers a workflow project over the cached project", async () => {
    localStorage.setItem(
      "flow.selection.v1",
      JSON.stringify({ workflowIntent: "saved-workflow", projectId: "proj-1" }),
    );

    render(FlowView);

    const workflowSelect = (await screen.findByLabelText(
      "Workflow",
    )) as HTMLSelectElement;
    const projectSelect = (await screen.findByLabelText(
      "Project",
    )) as HTMLSelectElement;
    await waitFor(() => {
      expect(workflowSelect.value).toBe("saved-workflow");
      expect(projectSelect.value).toBe("proj-2");
    });

    await fireEvent.update(projectSelect, "proj-1");
    await waitFor(() => {
      expect(
        JSON.parse(localStorage.getItem("flow.selection.v1") || "{}").projectId,
      ).toBe("proj-1");
    });
  });

  it("shows runtime values in run mode after execution", async () => {
    render(FlowView);

    const runBtn = await screen.findByRole("button", {
      name: "Run workflow",
    });
    await waitFor(() => {
      expect((runBtn as HTMLButtonElement).disabled).toBe(false);
    });
    await fireEvent.click(runBtn);

    const viewButtons = await screen.findAllByText("View details");
    expect(viewButtons.length).toBeGreaterThan(0);
    await fireEvent.click(viewButtons[0]);
    await waitFor(() => {
      expect(screen.getByText("Delta")).toBeTruthy();
      expect(screen.getByText(/"result": "ok"/)).toBeTruthy();
    });

    const designToggle = screen.getByRole("button", { name: "Design" });
    await fireEvent.click(designToggle);
    expect(await screen.findByLabelText("Textbox Content")).toBeTruthy();
  });
});
