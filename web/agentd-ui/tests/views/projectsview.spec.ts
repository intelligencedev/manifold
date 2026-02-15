import { fireEvent, render, waitFor } from "@testing-library/vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsView from "@/views/ProjectsView.vue";

const apiMocks = vi.hoisted(() => ({
  deleteProject: vi.fn(async () => {}),
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
  listProjectTree: async () => [],
  uploadFile: async () => {},
  deletePath: async () => {},
  createDir: async () => {},
  moveProjectPath: async () => {},
  fetchProjectFileText: async () => "",
  saveProjectFileText: async () => {},
  setActiveProject: async () => {},
  getUserPreferences: async () => ({ activeProjectId: "proj-1" }),
  projectFileUrl: (id: string, path: string) =>
    `/api/projects/${encodeURIComponent(id)}/files?path=${encodeURIComponent(path)}`,
  projectArchiveUrl: (id: string) =>
    `/api/projects/${encodeURIComponent(id)}/archive`,
}));

describe("ProjectsView", () => {
  beforeEach(() => {
    apiMocks.deleteProject.mockClear();
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
});
