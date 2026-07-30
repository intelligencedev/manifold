import { fireEvent, render, waitFor } from "@testing-library/vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsView from "@/views/ProjectsView.vue";

const apiMocks = vi.hoisted(() => ({
  deleteProject: vi.fn(async () => {}),
  fetchProjectFileText: vi.fn(async (_id: string, path: string) =>
    path === "README.md"
      ? "# Diagram\n\n```mermaid\ngraph TD\n  A-->B\n```"
      : "",
  ),
  projectFileUrl: vi.fn(
    (id: string, path: string) =>
      `/api/projects/${encodeURIComponent(id)}/files?path=${encodeURIComponent(path)}`,
  ),
  projectArchiveUrl: vi.fn((id: string, path?: string) => {
    if (!path) return `/api/projects/${encodeURIComponent(id)}/archive`;
    return `/api/projects/${encodeURIComponent(id)}/archive?path=${encodeURIComponent(path)}`;
  }),
}));

const mermaidMocks = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn(async () => ({
    svg: '<svg viewBox="0 0 100 50"><g>diagram</g><script>bad()</script></svg>',
  })),
}));

vi.mock("@/api/client", () => ({
  listProjects: async () => [
    {
      id: "proj-1",
      name: "codeqa",
      createdAt: "2026-02-12T12:00:00Z",
      updatedAt: "2026-02-12T12:00:00Z",
      sizeBytes: 44100,
      files: 5,
    },
  ],
  createProject: async (name: string) => ({
    id: "proj-2",
    name,
    createdAt: "2026-02-13T12:00:00Z",
    updatedAt: "2026-02-13T12:00:00Z",
    sizeBytes: 0,
    files: 0,
  }),
  deleteProject: apiMocks.deleteProject,
  listProjectTree: async (_id: string, path = ".") => {
    if (!path || path === ".") {
      return [
        {
          name: "src",
          path: "src",
          isDir: true,
          sizeBytes: 0,
          modTime: "2026-02-13T12:00:00Z",
        },
        {
          name: "README.md",
          path: "README.md",
          isDir: false,
          sizeBytes: 56,
          modTime: "2026-02-13T12:00:00Z",
        },
      ];
    }
    return [];
  },
  uploadFile: async () => {},
  deletePath: async () => {},
  createDir: async () => {},
  moveProjectPath: async () => {},
  fetchProjectFileText: apiMocks.fetchProjectFileText,
  saveProjectFileText: async () => {},
  setActiveProject: async () => {},
  getUserPreferences: async () => ({ activeProjectId: "proj-1" }),
  projectFileUrl: apiMocks.projectFileUrl,
  projectArchiveUrl: apiMocks.projectArchiveUrl,
}));

vi.mock("mermaid", () => ({
  default: mermaidMocks,
}));

describe("ProjectsView", () => {
  beforeEach(() => {
    apiMocks.deleteProject.mockClear();
    apiMocks.fetchProjectFileText.mockClear();
    apiMocks.projectFileUrl.mockClear();
    apiMocks.projectArchiveUrl.mockClear();
    mermaidMocks.initialize.mockClear();
    mermaidMocks.render.mockClear();
  });

  it("requires explicit typed confirmation before deleting a project", async () => {
    const { findByRole, getByLabelText, getByRole } = render(ProjectsView);

    const openDeleteDialog = await findByRole("button", {
      name: /Delete current project/i,
    });
    await fireEvent.click(openDeleteDialog);

    const typedNameInput = getByLabelText(
      /Type project name to confirm/i,
    ) as HTMLInputElement;
    const acknowledge = getByLabelText(/Acknowledge project deletion/i);
    const deleteButton = getByRole("button", {
      name: /^Delete Project$/i,
    }) as HTMLButtonElement;

    expect(deleteButton).toBeDisabled();

    await fireEvent.update(typedNameInput, "wrong-name");
    await fireEvent.click(acknowledge);
    expect(deleteButton).toBeDisabled();

    await fireEvent.update(typedNameInput, "codeqa");
    expect(deleteButton).toBeEnabled();

    await fireEvent.click(deleteButton);
    await waitFor(() => {
      expect(apiMocks.deleteProject).toHaveBeenCalledTimes(1);
      expect(apiMocks.deleteProject).toHaveBeenCalledWith("proj-1");
    });
  });

  it("downloads selected folders as archives", async () => {
    const { findByLabelText, findByRole } = render(ProjectsView);

    const folderCheckbox = await findByLabelText(/Select src/i);
    await fireEvent.click(folderCheckbox);

    const downloadSelected = (await findByRole("button", {
      name: /Download Selected/i,
    })) as HTMLButtonElement;

    await waitFor(() => {
      expect(downloadSelected).toBeEnabled();
    });

    await fireEvent.click(downloadSelected);

    await waitFor(() => {
      expect(apiMocks.projectArchiveUrl).toHaveBeenCalledWith("proj-1", "src");
    });
    expect(apiMocks.projectFileUrl).not.toHaveBeenCalledWith("proj-1", "src");
  });

  it("renders mermaid fences in markdown preview", async () => {
    const { findByText, findByRole, container } = render(ProjectsView);

    await fireEvent.click(await findByText("README.md"));
    await fireEvent.click(
      await findByRole("button", {
        name: "Markdown",
      }),
    );

    await waitFor(() => {
      expect(mermaidMocks.initialize).toHaveBeenCalledWith(
        expect.objectContaining({
          securityLevel: "strict",
          startOnLoad: false,
          theme: "base",
          htmlLabels: false,
          themeVariables: expect.objectContaining({
            nodeTextColor: expect.any(String),
            primaryTextColor: expect.any(String),
            textColor: expect.any(String),
          }),
        }),
      );
      expect(mermaidMocks.render).toHaveBeenCalledWith(
        expect.stringMatching(/^project-mermaid-/),
        expect.stringContaining("graph TD"),
      );
    });

    await waitFor(() => {
      expect(container.querySelector(".md-mermaid-rendered svg")).toBeTruthy();
    });
    expect(container.querySelector(".md-mermaid-rendered script")).toBeNull();
  });
});
