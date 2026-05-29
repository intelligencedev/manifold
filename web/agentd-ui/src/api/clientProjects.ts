import { apiClient, baseURL } from "./clientCore";

export interface ProjectSummary {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
  sizeBytes: number;
  files: number;
}

export interface FileEntry {
  name: string;
  path: string;
  isDir: boolean;
  sizeBytes: number;
  modTime: string;
}

export async function listProjects(): Promise<ProjectSummary[]> {
  const { data } = await apiClient.get<{ projects: ProjectSummary[] }>(
    "/projects",
  );
  return data.projects || [];
}

export async function createProject(name: string): Promise<ProjectSummary> {
  const { data } = await apiClient.post<ProjectSummary>("/projects", { name });
  return data;
}

export async function deleteProject(id: string): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(id)}`);
}

export async function listProjectTree(
  id: string,
  path = ".",
): Promise<FileEntry[]> {
  const { data } = await apiClient.get<{ entries: FileEntry[] }>(
    `/projects/${encodeURIComponent(id)}/tree`,
    { params: path ? { path } : undefined },
  );
  return data.entries || [];
}

export async function createDir(id: string, path: string): Promise<void> {
  await apiClient.post(`/projects/${encodeURIComponent(id)}/dirs`, null, {
    params: { path },
  });
}

export async function deletePath(id: string, path: string): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(id)}/files`, {
    params: { path },
  });
}

export async function moveProjectPath(
  id: string,
  from: string,
  to: string,
): Promise<void> {
  await apiClient.post(`/projects/${encodeURIComponent(id)}/move`, {
    from,
    to,
  });
}

export async function uploadFile(
  id: string,
  dirPath: string,
  file: File,
  name?: string,
): Promise<void> {
  const form = new FormData();
  form.append("file", file, file.name);
  if (name) form.append("name", name);
  await apiClient.post(`/projects/${encodeURIComponent(id)}/files`, form, {
    params: { path: dirPath, name },
  });
}

export async function fetchProjectFileText(
  id: string,
  path: string,
): Promise<string> {
  const { data } = await apiClient.get<string>(
    `/projects/${encodeURIComponent(id)}/files`,
    {
      params: { path },
      responseType: "text",
      transformResponse: (raw) => raw,
    },
  );
  return typeof data === "string" ? data : String(data ?? "");
}

export async function saveProjectFileText(
  id: string,
  dirPath: string,
  name: string,
  content: string,
): Promise<void> {
  await apiClient.post(`/projects/${encodeURIComponent(id)}/files`, content, {
    params: { path: dirPath, name },
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
}

export function projectFileUrl(id: string, path: string): string {
  const b = baseURL.replace(/\/$/, "");
  const qp = new URLSearchParams({ path }).toString();
  return `${b}/projects/${encodeURIComponent(id)}/files?${qp}`;
}

export function projectArchiveUrl(id: string, path?: string): string {
  const b = baseURL.replace(/\/$/, "");
  const cleanPath = path?.trim();
  if (!cleanPath) {
    return `${b}/projects/${encodeURIComponent(id)}/archive`;
  }
  const qp = new URLSearchParams({ path: cleanPath }).toString();
  return `${b}/projects/${encodeURIComponent(id)}/archive?${qp}`;
}
