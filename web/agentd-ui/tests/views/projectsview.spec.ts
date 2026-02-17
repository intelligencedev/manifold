import { fireEvent, render, waitFor } from "@testing-library/vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsView from "@/views/ProjectsView.vue";

const apiMocks = vi.hoisted(() => ({
  deleteProject: vi.fn(async () => {}),
  projectFileUrl: vi.fn((id: string, path: string) =>
    `/api/projects/${encodeURIComponent(id)}/files?path=${encodeURIComponent(path)}`,
  ),
  projectArchiveUrl: vi.fn((id: string, path?: string) => {
    if (!path) return `/api/projects/${encodeURIComponent(id)}/archive`;
    return `/api/projects/${encodeURIComponent(id)}/archive?path=${encodeURIComponent(path)}`;
  }),
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
      ];
    }
    return [];
  },
  uploadFile: async () => {},
  deletePath: async () => {},
  createDir: async () => {},
  moveProjectPath: async () => {},
  fetchProjectFileText: async () => "",
  saveProjectFileText: async () => {},
  setActiveProject: async () => {},
  getUserPreferences: async () => ({ activeProjectId: "proj-1" }),
  projectFileUrl: apiMocks.projectFileUrl,
  projectArchiveUrl: apiMocks.projectArchiveUrl,
}));

describe("ProjectsView", () => {
  beforeEach(() => {
    apiMocks.deleteProject.mockClear();
    apiMocks.projectFileUrl.mockClear();
    apiMocks.projectArchiveUrl.mockClear();
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
    const acknowledge = getByRole("checkbox");
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
});
