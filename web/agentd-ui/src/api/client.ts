import axios from "axios";

const baseURL = import.meta.env.VITE_AGENT_API_BASE_URL || "/api";

export const apiClient = axios.create({
  baseURL,
  timeout: 30_000,
  withCredentials: true,
});

export interface AgentStatus {
  id: string;
  name: string;
  state: "online" | "offline" | "degraded";
  model: string;
  updatedAt: string;
}

export async function fetchAgentStatus(): Promise<AgentStatus[]> {
  const response = await apiClient.get<AgentStatus[]>("/status");
  return response.data;
}

export interface AgentRun {
  id: string;
  prompt: string;
  createdAt: string;
  status: "running" | "failed" | "completed";
  tokens?: number;
}

export async function fetchAgentRuns(): Promise<AgentRun[]> {
  const response = await apiClient.get<AgentRun[]>("/runs");
  return response.data;
}

export interface TokenMetricsRow {
  model: string;
  prompt: number;
  completion: number;
  total: number;
}

export interface TokenMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  models: TokenMetricsRow[];
}

export interface TokenMetricsParams {
  window?: string;
  windowSeconds?: number;
}

export async function fetchTokenMetrics(
  params?: TokenMetricsParams,
): Promise<TokenMetricsResponse> {
  const response = await apiClient.get<TokenMetricsResponse>(
    "/metrics/tokens",
    {
      params,
    },
  );
  return response.data;
}

export interface TraceMetricRow {
  traceId?: string;
  name: string;
  model?: string;
  status: string;
  durationMillis?: number;
  timestamp: number;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
}

export interface TraceMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  traces: TraceMetricRow[];
}

export interface TraceMetricsParams {
  window?: string;
  windowSeconds?: number;
  limit?: number;
}

export async function fetchTraceMetrics(
  params?: TraceMetricsParams,
): Promise<TraceMetricsResponse> {
  const response = await apiClient.get<TraceMetricsResponse>(
    "/metrics/traces",
    {
      params,
    },
  );
  return response.data;
}

export interface LogMetricsRow {
  timestamp: number;
  level: string;
  message: string;
  service?: string;
  traceId?: string;
  spanId?: string;
}

export interface LogMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  logs: LogMetricsRow[];
}

export interface LogMetricsParams {
  window?: string;
  windowSeconds?: number;
  limit?: number;
}

export async function fetchLogMetrics(
  params?: LogMetricsParams,
): Promise<LogMetricsResponse> {
  const response = await apiClient.get<LogMetricsResponse>("/metrics/logs", {
    params,
  });
  return response.data;
}

// Projects API --------------------------------------------------------------

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
  await apiClient.post(
    `/projects/${encodeURIComponent(id)}/files`,
    content,
    {
      params: { path: dirPath, name },
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
      },
    },
  );
}

// Build a direct URL to fetch a file's content for preview/download.
export function projectFileUrl(id: string, path: string): string {
  const b = baseURL.replace(/\/$/, "");
  const qp = new URLSearchParams({ path }).toString();
  return `${b}/projects/${encodeURIComponent(id)}/files?${qp}`;
}

// Build a URL to download the entire project as a tar.gz archive.
export function projectArchiveUrl(id: string): string {
  const b = baseURL.replace(/\/$/, "");
  return `${b}/projects/${encodeURIComponent(id)}/archive`;
}

// User Preferences API -------------------------------------------------------

export interface UserPreferences {
  userId: number;
  activeProjectId?: string;
  updatedAt: string;
}

export async function getUserPreferences(): Promise<UserPreferences | null> {
  try {
    const { data } = await apiClient.get<UserPreferences>("/me/preferences");
    return data;
  } catch (e: any) {
    // If 404 or not found, return null (no preferences set yet)
    if (e?.response?.status === 404) return null;
    throw e;
  }
}

export async function setActiveProject(projectId: string): Promise<void> {
  await apiClient.post("/me/preferences/project", { projectId });
}

// Specialists CRUD
export interface Specialist {
  id?: number;
  name: string;
  description?: string;
  provider?: string;
  baseURL: string;
  apiKey?: string;
  model: string;
  summaryContextWindowTokens?: number;
  enableTools: boolean;
  paused: boolean;
  allowTools?: string[];
  system?: string;
  extraHeaders?: Record<string, string>;
  extraParams?: Record<string, any>;
  teams?: string[];
}

export interface SpecialistTeam {
  id?: number;
  userId?: number;
  name: string;
  description?: string;
  orchestrator: Specialist;
  members: string[];
  createdAt?: string;
  updatedAt?: string;
}

export interface SpecialistProviderDefaults {
  provider: string;
  baseURL: string;
  apiKey?: string;
  model: string;
  extraHeaders?: Record<string, string>;
  extraParams?: Record<string, any>;
}

export async function listSpecialists(): Promise<Specialist[]> {
  const { data } = await apiClient.get<Specialist[]>("/specialists");
  return data;
}

export async function getSpecialist(name: string): Promise<Specialist> {
  const { data } = await apiClient.get<Specialist>(
    `/specialists/${encodeURIComponent(name)}`,
  );
  return data;
}

export async function upsertSpecialist(sp: Specialist): Promise<Specialist> {
  // POST for create, PUT for update by name
  if (sp.name && sp.id == null) {
    const { data } = await apiClient.post<Specialist>("/specialists", sp);
    return data;
  }
  const { data } = await apiClient.put<Specialist>(
    `/specialists/${encodeURIComponent(sp.name)}`,
    sp,
  );
  return data;
}

export async function deleteSpecialist(name: string): Promise<void> {
  await apiClient.delete(`/specialists/${encodeURIComponent(name)}`);
}

export async function listSpecialistDefaults(): Promise<
  Record<string, SpecialistProviderDefaults>
> {
  const { data } = await apiClient.get<
    Record<string, SpecialistProviderDefaults>
  >("/specialists/defaults");
  return data;
}

// Specialist Teams
export async function listTeams(): Promise<SpecialistTeam[]> {
  const { data } = await apiClient.get<SpecialistTeam[]>("/teams");
  return data;
}

export async function getTeam(name: string): Promise<SpecialistTeam> {
  const { data } = await apiClient.get<SpecialistTeam>(
    `/teams/${encodeURIComponent(name)}`,
  );
  return data;
}

export async function upsertTeam(
  team: SpecialistTeam,
): Promise<SpecialistTeam> {
  if (team.name && team.id == null) {
    const { data } = await apiClient.post<SpecialistTeam>("/teams", team);
    return data;
  }
  const { data } = await apiClient.put<SpecialistTeam>(
    `/teams/${encodeURIComponent(team.name)}`,
    team,
  );
  return data;
}

export async function deleteTeam(name: string): Promise<void> {
  await apiClient.delete(`/teams/${encodeURIComponent(name)}`);
}

export async function addTeamMember(
  teamName: string,
  specialistName: string,
): Promise<void> {
  await apiClient.put(
    `/teams/${encodeURIComponent(teamName)}/members/${encodeURIComponent(
      specialistName,
    )}`,
  );
}

export async function removeTeamMember(
  teamName: string,
  specialistName: string,
): Promise<void> {
  await apiClient.delete(
    `/teams/${encodeURIComponent(teamName)}/members/${encodeURIComponent(
      specialistName,
    )}`,
  );
}

// Users & Roles
export interface User {
  id: number;
  email: string;
  name: string;
  picture?: string;
  provider?: string;
  subject?: string;
  roles: string[];
}

export async function listUsers(): Promise<User[]> {
  const { data } = await apiClient.get<User[]>("/users");
  return data;
}

export async function createUser(u: Partial<User>): Promise<User> {
  const { data } = await apiClient.post<User>("/users", u);
  return data;
}

export async function updateUser(id: number, u: Partial<User>): Promise<User> {
  const { data } = await apiClient.put<User>(`/users/${id}`, u);
  return data;
}

export async function deleteUser(id: number): Promise<void> {
  await apiClient.delete(`/users/${id}`);
}

// Agentd configuration ------------------------------------------------------

export interface AgentdSettings {
  openaiSummaryModel: string;
  openaiSummaryUrl: string;
  summaryEnabled: boolean;
  summaryThreshold: number;
  summaryKeepLast: number;

  embedBaseUrl: string;
  embedModel: string;
  embedApiKey: string;
  embedApiHeader: string;
  embedApiHeaders: Record<string, string>;
  embedPath: string;

  agentRunTimeoutSeconds: number;
  streamRunTimeoutSeconds: number;
  workflowTimeoutSeconds: number;

  blockBinaries: string;
  maxCommandSeconds: number;
  outputTruncateBytes: number;

  otelServiceName: string;
  serviceVersion: string;
  environment: string;
  otelExporterOtlpEndpoint: string;

  logPath: string;
  logLevel: string;
  logPayloads: boolean;

  searxngUrl: string;
  webSearxngUrl: string;

  databaseUrl: string;
  dbUrl: string;
  postgresDsn: string;

  searchBackend: string;
  searchDsn: string;
  searchIndex: string;

  vectorBackend: string;
  vectorDsn: string;
  vectorIndex: string;
  vectorDimensions: number;
  vectorMetric: string;

  graphBackend: string;
  graphDsn: string;
}

export async function fetchAgentdSettings(): Promise<AgentdSettings> {
  const { data } = await apiClient.get<AgentdSettings>("/config/agentd");
  return data;
}

export async function updateAgentdSettings(
  payload: AgentdSettings,
): Promise<AgentdSettings> {
  // Try PATCH -> PUT -> POST in order; treat 405/404/501 as "method not allowed/implemented" and fall through.
  const tryCall = async (method: "patch" | "put" | "post") => {
    switch (method) {
      case "patch":
        return apiClient.patch<AgentdSettings>("/config/agentd", payload);
      case "put":
        return apiClient.put<AgentdSettings>("/config/agentd", payload);
      case "post":
        return apiClient.post<AgentdSettings>("/config/agentd", payload);
    }
  };

  const methods: Array<"patch" | "put" | "post"> = ["patch", "put", "post"];
  let lastErr: any;
  for (const m of methods) {
    try {
      const { data } = await tryCall(m);
      return data;
    } catch (e: any) {
      const status = e?.response?.status;
      // If method not allowed/not implemented or not found, try next
      if (status === 405 || status === 404 || status === 501) {
        lastErr = e;
        continue;
      }
      // Other errors: propagate
      throw e;
    }
  }
  // If we exhausted all methods with 405/404/501, throw a friendlier error the UI can detect.
  const err: any = new Error(
    "Agentd configuration is read-only or no write endpoint is available",
  );
  err.code = "READ_ONLY";
  err.response = lastErr?.response;
  throw err;
}
