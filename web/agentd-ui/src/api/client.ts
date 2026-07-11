export { apiClient, baseURL } from "./clientCore";
import { apiClient } from "./clientCore";
export * from "./clientMetrics";
export * from "./clientProjects";

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

export interface AgentRunsParams {
  window?: string;
  windowSeconds?: number;
  limit?: number;
}

export async function fetchAgentRuns(
  params?: AgentRunsParams,
): Promise<AgentRun[]> {
  const response = await apiClient.get<AgentRun[]>("/runs", { params });
  return response.data;
}

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
    if (e?.response?.status === 404) return null;
    throw e;
  }
}

export async function setActiveProject(projectId: string): Promise<void> {
  await apiClient.post("/me/preferences/project", { projectId });
}

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
  requestInfoEnabled?: boolean | null;
  imageGeneration?: boolean;
  videoGeneration?: boolean;
  autoDiscover?: boolean | null;
  paused: boolean;
  allowTools?: string[];
  system?: string;
  promptId?: string;
  promptVersionId?: string;
  extraHeaders?: Record<string, string>;
  extraParams?: Record<string, any>;
  teams?: string[];
  harness?: SpecialistHarness | null;
}

export interface SpecialistHarness {
  enabled: boolean;
  mode: "legacy" | "guarded_chat" | "workflow" | string;
  rescueEnabled: boolean;
  maxRetriesPerStep: number;
  maxToolErrors: number;
  terminalTools: string[];
  requiredSteps: string[];
  toolPrerequisites: Record<string, SpecialistHarnessPrerequisite[]>;
  compact: SpecialistHarnessCompact;
}

export interface SpecialistHarnessPrerequisite {
  tool: string;
  matchArg?: string;
}

export interface SpecialistHarnessCompact {
  enabled: boolean;
  keepRecentSteps: number;
  phaseThresholds: number[];
}

export interface SpecialistTeam {
  id?: number;
  userId?: number;
  name: string;
  description?: string;
  orchestratorName?: string;
  orchestrator?: Specialist;
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
  if (team.name && (!Number.isFinite(team.id) || (team.id ?? 0) <= 0)) {
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

export interface AgentdSettings {
  serverConfig: Record<string, unknown>;
  configSource: string;
  configPatch: Record<string, unknown>;
  llmProvider: string;
  llmApiKey: string;
  llmModel: string;
  llmBaseUrl: string;
  memoryEnabled: boolean;

  openaiSummaryModel: string;
  openaiSummaryUrl: string;
  summaryProvider: string;
  summaryModel: string;
  summaryUrl: string;
  summaryEnabled: boolean;
  summaryContextWindowTokens: number;
  summaryPlainTextContextWindowTokens: number;
  summaryReserveBufferTokens: number;
  summaryMinKeepLastMessages: number;
  summaryMaxKeepLastMessages: number;
  summaryMaxSummaryChunkTokens: number;
  summaryCallTimeoutSeconds: number;
  summaryTokenBudget: number;
  requestInfoEnabled: boolean;

  promptBaseSystem: string;
  promptMemoryInstructions: string;
  promptToolDiscoveryInstructions: string;
  promptSkillDiscoveryInstructions: string;

  embedBaseUrl: string;
  embedModel: string;
  embedApiKey: string;
  embedApiHeader: string;
  embedApiHeaders: Record<string, string>;
  embedPath: string;
  embedInstructionMode: string;
  embedInstructionFormat: string;
  embedDefaultQueryInstruction: string;
  embedRagQueryInstruction: string;
  embedEvolvingMemoryQueryInstruction: string;
  embedTransitQueryInstruction: string;

  rerankEnabled: boolean;
  rerankBaseUrl: string;
  rerankModel: string;
  rerankInstruction: string;
  rerankApiKey: string;
  rerankApiHeader: string;
  rerankApiHeaders: Record<string, string>;
  rerankPath: string;

  agentRunTimeoutSeconds: number;
  streamRunTimeoutSeconds: number;
  workflowTimeoutSeconds: number;

  blockBinaries: string;
  sandboxEnabled: boolean | null;
  sandboxFailIfUnavailable: boolean | null;
  sandboxNetworkEnabled: boolean | null;
  sandboxNetworkAllowedDomains: string[];
  maxCommandSeconds: number;
  outputTruncateBytes: number;
  maxTerminalSessions: number;
  maxTerminalRuntimeSeconds: number;
  terminalIdleTTLSeconds: number;
  terminalOutputBufferBytes: number;

  otelServiceName: string;
  serviceVersion: string;
  environment: string;
  otelExporterOtlpEndpoint: string;

  logPath: string;
  logLevel: string;
  logPayloads: boolean;
  logRawPrompts: boolean;

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
  for (const method of methods) {
    try {
      const { data } = await tryCall(method);
      return data;
    } catch (e: any) {
      const status = e?.response?.status;
      if (status === 405 || status === 404 || status === 501) {
        lastErr = e;
        continue;
      }
      throw e;
    }
  }
  const err: any = new Error(
    "Agentd configuration is read-only or no write endpoint is available",
  );
  err.code = "READ_ONLY";
  err.response = lastErr?.response;
  throw err;
}

export interface SetupStatus {
  ready: boolean;
  needsSetup: boolean;
  provider: string;
  model: string;
  hasCredentials: boolean;
  memoryEnabled: boolean;
  embeddingRequired: boolean;
  configPath: string;
  baseUrl?: string;
  listenAddr?: string;
}

export interface SetupCompleteRequest {
  provider: string;
  apiKey: string;
  model?: string;
  baseUrl?: string;
  memoryEnabled?: boolean;
  embedApiKey?: string;
  embedBaseUrl?: string;
  embedModel?: string;
}

export async function fetchSetupStatus(): Promise<SetupStatus> {
  const { data } = await apiClient.get<SetupStatus>("/setup/status");
  return data;
}

export async function completeSetup(
  payload: SetupCompleteRequest,
): Promise<SetupStatus> {
  const { data } = await apiClient.post<SetupStatus>(
    "/setup/complete",
    payload,
  );
  return data;
}
